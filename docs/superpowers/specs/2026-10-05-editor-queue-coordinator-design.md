# Editor Queue Coordinator — thiết kế

Ngày: 2026-10-05. Repo: `unity-cli-agentkit`. Trạng thái: chờ review.

## 1. Vấn đề

Nhiều agent (hiện 7 tab) dùng chung một Unity Editor qua lock FIFO
(`skills/utk-cli-core/scripts/unity-lock.sh`, `unity-job.sh`). Mỗi lượt là một
job trọn gói: lấy lock → `utk editor refresh` (compile + domain reload) → chạy
test / builder / chụp ảnh → nhả lock. Job giữ tối đa 900 s, chờ tối đa 3600 s.
Với 7 agent, người cuối hàng chờ 20–35 phút ở mức trung bình và tới hàng giờ khi
có job chạm trần hoặc một agent mang lỗi compile vào Editor (sự cố đã ghi:
1 lỗi compile chặn 11 builder hàng giờ — `skills/unity-parallel-branch/SKILL.md:33`).

Thêm Editor bị loại vì RAM (máy 24 GB, mỗi Editor ~2 GB, cùng project không
mở được 2 process do `Temp/UnityLockfile`).

### Số liệu khảo sát

| Project | asmdef | .cs | Enter Play Mode Options | Scene chính |
|---|---|---|---|---|
| DU02 | 28 | 1671 | tắt → mỗi lần Play reload domain | GamePlay: 1436 GO / 1291 MonoBehaviour / 58 PrefabInstance (8.8 MB) |
| DU04 | 20 | 1464 | tắt | GamePlay: 764 GO / 909 MB / 59 PI |
| Paper_Doll | 23 | 943 | tắt | — |

Scene **không mỏng**: logic nằm trực tiếp trong scene, agent sẽ còn sửa scene
thường xuyên. Thiết kế phải chạy được với scene béo.

Các sự thật quyết định thiết kế (từ đọc code `utk` và tài liệu Unity):
- Editor xử lý mọi lệnh tuần tự trên main thread; pipeline server là single
  request loop. Không có lớp lệnh "read-only song song".
- Phần đắt nhất của một job là compile + domain reload, không phải chạy test.
  7 job = 7 lần compile + reload dù một lần là đủ cho mọi thay đổi.
- Unity Test Framework nhận nhiều filter trong một run (`-testFilter "A;B"`
  hoặc regex); kết quả là NUnit XML, tách theo `FullName` được.
- `utk run_tests` đã có `foreignTest` khớp `Results[].FullName` với filter
  (`cmd/utk/tests.go:167-182`) — tái dùng để tách kết quả.
- Vào Play khi Domain Reload bật trả thêm một reload. Unity công bố giảm 50–90 %
  thời gian vào Play khi tắt reload domain; rủi ro là static state rò giữa các
  lần Play (SDU đã dính).
- Offline test (`unity-test.sh offline --tests`) chỉ phủ ~9 % test của game
  UI-heavy; không phải lối thoát chính trong ngắn hạn.
- Chưa có telemetry nào về thời gian chờ / giữ lock.

## 2. Mục tiêu và ngoài phạm vi

Mục tiêu:
1. Chi phí cố định (compile, reload, vào Play) trả **một lần mỗi chu kỳ** cho
   mọi agent, thay vì mỗi agent một lần.
2. Lỗi compile của một agent không chặn agent khác.
3. Job ngắn không chờ sau job dài.
4. Có số liệu chờ / giữ theo loại job để kiểm chứng từng bước.

Ngoài phạm vi:
- Thêm Editor / worktree (xem `unity-parallel-branch`).
- Chuyển scene sang "mỏng" (việc của code game, nhiều tuần). Ghi là khuyến
  nghị dài hạn ở §9.
- Đưa thêm test ra offline (cần refactor code game).
- Fairness giữa agent; FIFO giữa các chu kỳ là đủ cho 7 agent.
- Chen lệnh đọc vào giữa chu kỳ: Editor tự tuần tự hóa, agent gọi thẳng
  `utk exec`/`batch`, không qua coordinator.

## 3. Nguyên tắc: chỉ lock cái toàn cục

| Loại việc | Trạng thái chạm | Qua coordinator? | Cách chạy |
|---|---|---|---|
| Sửa prefab (builder) | File của mình | Không | 1 `utk exec` nguyên tử: `LoadPrefabContents` → sửa → `SaveAsPrefabAsset` → `UnloadPrefabContents` → `ImportAsset(ForceSynchronousImport)` (mẫu trong `utk-asset-edit`). Tranh chấp file giải quyết bằng sở hữu: mỗi task gán prefab/thư mục riêng, ghi trên task board. |
| Query, console, kiểm tra UI trên prefab | Đọc | Không | `utk exec`/`batch`; layout check bằng `NewPreviewScene` + instantiate prefab trong Edit mode. |
| Compile / refresh | Toàn cục | Có, gộp | 1 lần mỗi chu kỳ, sau cổng offline. |
| Test | 1 run / Editor | Có, gộp | 1 run với filter hợp, tách kết quả. |
| Sửa scene, `OpenScene`, ghi nhiều asset liên quan | Scene đang mở là của chung | Có, độc quyền | Tuần tự, ngắn trước. |
| Play mode (screenshot runtime, UI check runtime, PlayMode test) | Toàn cục | Có, gộp | 1 phiên Play phục vụ mọi yêu cầu rồi thoát. |

## 4. Kiến trúc

```
Agent (N tab)
  │  utk queue submit <kind> [...]   chặn tới khi có kết quả của mình
  │  utk exec / batch                việc không cần hàng đợi, gọi thẳng
  ▼
utk queue serve   1 process / Editor; submit đầu tiên tự spawn nếu chưa chạy
  │  đọc  $QDIR/requests/<id>.json
  │  chạy chu kỳ: gate → refresh → tests → scene jobs → shot batch
  │  ghi   $QDIR/results/<id>.json ; telemetry.jsonl
  ▼
unity-lock.sh (giữ nguyên)   coordinator là người giữ lock trong lúc chạy chu kỳ
  ▼
Unity Editor (1)
```

- `$QDIR = ~/.unity-cli-agentkit/queue/<UNITY_LOCK_NAME>` (mặc định
  `unity-editor`). Một queue cho một Editor, cùng cách định danh với lock.
- Hàng đợi là **file JSON**, không socket: khớp `unity-lock.sh`, sống sót khi
  coordinator chết, `ls` là debug được.
- **Lock không bỏ.** Coordinator `acquire` đầu chu kỳ, `release` cuối chu kỳ.
  Agent còn dùng `unity-job.sh` kiểu cũ vẫn chạy, chỉ xếp sau chu kỳ.
  Chuyển dần, không big-bang.
- Coordinator viết bằng Go trong `cmd/utk/` (`queue.go`, `queue_cycle.go`,
  `queue_test.go`), lệnh `utk queue serve|submit|status|stats`. Lý do chọn
  Go thay bash: tách NUnit XML tái dùng code `tests.go`, test bằng `go test`
  như toàn repo, `utk` đã có trong PATH mọi project, timeout/process group
  trong Go gọn hơn bash.

### 4.1 Request

```json
{
  "id": "20261005-143012-7f3a",
  "kind": "test",            // compile | test | scene | shot
  "agent": "tab3",           // --agent, mặc định $UNITY_LOCK_OWNER hoặc "pid-<n>"
  "pid": 48213,              // pid của `submit`; pid chết → request bị bỏ
  "cwd": "/Users/.../DU02",  // project root; coordinator chạy utk với cwd này
  "submitted": "2026-10-05T14:30:12+07:00",
  "args": { ... }            // theo kind, xem §5
}
```

`submit` ghi `requests/<id>.json.tmp` rồi `rename` → `requests/<id>.json`
(atomic). Sau đó poll `results/<id>.json` mỗi 1 s tới khi có hoặc tới
`--wait` (mặc định 3600 s). Có kết quả: in `stdout` của result, thoát với
`exit` của result, xóa cả hai file.

### 4.2 Result

```json
{ "id": "...", "status": "PASS|FAIL|TIMEOUT|GATE_FAILED|NO_TESTS_MATCHED|EDITOR_BLOCKED",
  "exit": 0, "stdout": "...", "artifacts": ["Temp/shots/a.png"],
  "wait_s": 41, "run_s": 95 }
```

## 5. Loại yêu cầu

| kind | args | gộp | kết quả |
|---|---|---|---|
| `compile` | — | 1 `utk editor refresh` cho cả chu kỳ | `COMPILE OK` hoặc toàn bộ lỗi CS (không cố phân lỗi theo agent; §6 gate đã lộ người gây lỗi) |
| `test` | `filter` (testName, bắt buộc), `mode` editmode\|playmode (mặc định editmode) | 1 `run_tests` với filter hợp `A;B;C` cho mỗi mode. Fallback nếu pipeline không nhận `;`: chạy tuần tự từng filter **dưới cùng một refresh** (vẫn tiết kiệm compile + reload, phần đắt nhất). | Kết quả lọc theo `FullName` chứa `filter` của agent; `PASS` nếu mọi test khớp pass. |
| `scene` | `cmd` (mảng args `utk`) hoặc `script` (file bash), `est_s` (mặc định 60), `timeout_s` (mặc định 300) | Không gộp. Tuần tự, **ngắn trước** theo `est_s`. | stdout + exit của lệnh. |
| `shot` | `script` (file bash, bắt buộc), `scene` (path, bắt buộc), `isolated` (bool), `edit` (bool) | 1 phiên Play cho mọi `shot` không `isolated`, nhóm theo `scene`. `isolated` → phiên Play riêng, chạy sau batch. `edit` → không vào Play, chạy script ở Edit mode. | stdout + exit; `artifacts` = file script in ra sau `ARTIFACT:`. |

`shot.script` chứa đúng các lệnh `utk` agent đang dùng (`exec`, `wait_for`,
`simulate_pointer`, `screenshot`). Coordinator cấp môi trường:
`UNITY_IN_PLAY=1` (script không được tự `editor_play`/`editor_stop`),
`UNITY_LOCK_OWNER=coordinator`. Trước mỗi script coordinator
`SceneManager.LoadScene(<scene>)` qua `utk exec` để script sau không thừa
hưởng panel đang mở của script trước.

Giới hạn nói rõ trong skill: trong cùng phiên Play, **static state rò giữa các
script** (như khi tắt domain reload). Cần sạch → `--isolated`.

## 6. Một chu kỳ

```
0. requests đang có → snapshot R (đến sau → chu kỳ kế); bỏ request có pid chết
1. gate: nếu R có compile|test → unity-test.sh offline <cwd>
     FAIL → mọi compile|test trong R nhận GATE_FAILED + lỗi CS;
            scene|shot vẫn chạy (Editor còn assembly cũ, dùng được); bỏ bước 3–4
2. unity-lock.sh acquire coordinator 3600 ; utk set_autotick --enable true
3. utk editor refresh (1 lần)        → FAIL: compile|test nhận lỗi; cảnh báo "gate sai"
4. tests: editmode gộp → tách ; playmode gộp → tách
5. scene jobs tuần tự, sort theo est_s tăng dần, mỗi job timeout riêng
6. shot: nhóm theo scene → mỗi nhóm 1 phiên Play (editor_play, chờ scene ready,
   với mỗi script: LoadScene → chạy script) → editor_stop ; rồi các isolated ;
   rồi các edit (không Play)
7. utk set_autotick --enable false ; unity-lock.sh release coordinator
8. ghi results + telemetry ; R rỗng → ngủ 2 s ; lặp
```

Thứ tự 4 → 5 → 6 cố định vì test và scene job chạy ở Edit mode, Play ở cuối
để không phải vào/ra Play hai lần. Compile chạy trước test và scene job để cả
hai dùng assembly mới.

## 7. Xử lý lỗi

| Tình huống | Xử lý |
|---|---|
| Gate FAIL | Như §6 bước 1. Request không bị xóa khỏi hàng; agent sửa xong `submit` lại, id mới thay id cũ cùng agent + kind + filter. |
| `refresh` FAIL dù gate OK (csproj lệch, file mới `NOT-IN-CSPROJ`) | compile\|test nhận lỗi; log `WARN gate-mismatch`. |
| Một filter không match test nào | Agent đó nhận `NO_TESTS_MATCHED`; agent khác không ảnh hưởng. |
| Run gộp `TEST_RUN_CRASHED` | Chạy lại tuần tự từng filter trong cùng chu kỳ; filter nào crash nhận `FAIL` kèm console. |
| Scene job quá `timeout_s` | TERM → KILL process group (tái dùng cơ chế perl `setpgrp` của `unity-job.sh`, viết lại bằng `syscall.SysProcAttr{Setpgid}` trong Go); result `TIMEOUT`; chu kỳ tiếp tục. |
| Script `shot` exit ≠ 0 | Chỉ agent đó `FAIL`; coordinator `LoadScene` cho script kế. Script treo quá 300 s → TIMEOUT, `editor_stop`, phiên Play mới cho phần còn lại. |
| Modal dialog | Không có lệnh đóng (`utk list` 2026-10-04). Khi `utk status` không trả lời quá 60 s: mọi request trong R nhận `EDITOR_BLOCKED`, coordinator ngừng nhận việc, poll `utk status` mỗi 10 s tới khi OK. |
| Coordinator chết giữa chu kỳ | Lock stale sau 15 phút (có sẵn). `submit` kế thấy pid trong `$QDIR/serve.pid` chết → spawn `serve` mới; request còn trong dir được chạy lại. |
| Agent chết khi chờ | Bước 0 bỏ request có pid chết; nếu đang chạy thì vẫn chạy xong, result bị xóa sau 1 giờ. |
| Hai `serve` cùng khởi động | `serve` lấy `mkdir $QDIR/serve.lock.d`; thua → thoát im lặng. |

## 8. Bước 0–1: telemetry và compile gate trong `unity-job.sh`

Độc lập với coordinator, làm trước để có số liệu và chặn kẹt compile ngay.

- `--kind compile|test|scene|shot|other` (mặc định `other`).
- Ghi một dòng JSON vào `~/.unity-cli-agentkit/telemetry.jsonl`:
  `{"ts","owner","task","kind","wait_s","hold_s","result"}`. Coordinator ghi
  cùng file, cùng schema (thêm `"via":"queue"`).
- `--gate <repo>`: chạy `unity-test.sh offline <repo>` trước `acquire`;
  không `COMPILE OK` → in lỗi, exit 1, không vào hàng.
- `utk queue stats [--since 24h]`: đọc telemetry, in p50/p95 `wait_s`, `hold_s`
  theo `kind`, số lần gate chặn.
- Agents-block (`internal/initcmd/agents-block.md`) và `utk-cli-core/SKILL.md`:
  job compile/test luôn kèm `--gate`; mọi job kèm `--kind`.

## 9. Bước 3b: Enter Play Mode Options (ở project game, ngoài repo này)

Quy trình, làm từng project, DU04 trước (nhỏ hơn) rồi DU02:
1. Project Settings → Editor → bật Enter Play Mode Options, **chỉ tắt Reload
   Domain**, giữ Reload Scene.
2. Rà static: grep `static [^(]*=|static event|static (List|Dictionary|HashSet)`
   trong `Assets/` (trừ `Plugins/`, `ThirdParty/`); mỗi hit cần reset trong
   `[RuntimeInitializeOnLoadMethod(RuntimeInitializeLoadType.SubsystemRegistration)]`
   hoặc ghi lý do vô hại.
3. 5 lần Play liên tiếp qua `shot` batch, so ảnh với baseline chụp trước khi bật.
4. Lỗi → tắt lại, ghi hit chưa xử lý, không giữ nửa vời.

Khuyến nghị dài hạn (không trong plan): tách feature trong scene ra prefab để
builder không còn phải sửa scene; mỗi lần làm feature mới thì làm theo
prefab-first, không migrate ồ ạt.

## 10. Kiểm thử

- Go `cmd/utk/queue_test.go`, Editor giả = binary stub qua `UTK_UNITY_BIN` trả
  JSON cố định theo args (pattern đã có trong `tests_test.go`, `recompile_test.go`):
  gộp filter đúng chuỗi; tách XML theo `FullName`; `NO_TESTS_MATCHED`; gate
  fail → đúng status cho đúng kind, scene/shot vẫn chạy; SJF theo `est_s`;
  request pid chết bị bỏ; crash → fallback tuần tự; timeout giết process
  group; hai `serve` → một thoát.
- Bash `unity-job.sh --selftest` thêm: `--kind` ghi đúng dòng telemetry;
  `--gate` chặn khi build fail (repo giả có lỗi CS) và cho qua khi OK.
- Chạy thật trên DU04 với 3 `submit test` song song: số lần `refresh` = 1;
  thời gian so với 3 `unity-job.sh` tuần tự. Ghi con số vào §11.

## 11. Baseline (điền sau khi chạy thật)

| Phép đo | Trước | Sau |
|---|---|---|
| 3 agent test song song, tổng thời gian | 29 s (3 job `unity-job.sh` tuần tự, mỗi job sửa 1 file + refresh + run_tests, hold 6 s/job) | 16 s (3 `utk queue submit` song song; wait 4 s cho cả ba, hold 6/8/10 s cộng dồn trong cycle) |
| Số lần compile + reload | 3 | 1 (cả ba cùng một cycle — cùng `ts`, cùng `wait_s`) |
| p95 wait_s theo kind sau 1 tuần | | (mẫu 3 request: p95 = 4 s; điền lại sau 1 tuần dùng thật bằng `utk queue stats --since 168h`) |

Đo trên DU04 với 3 class test probe nhỏ (`QueueProbe{A,B,C}Tests`, đã xoá sau khi đo), compile rất nhanh vì assembly nhỏ; với assembly game thật, phần tiết kiệm (2 compile + reload) lớn hơn nhiều so với con số trên.

Kiểm chứng trên DU04 (Unity 6000.0.81f1, com.unity.pipeline 0.8.0-exp.1, 2026-10-05):

- **Gộp filter `"A;B"`: KHÔNG được.** `run_tests --filter` của pipeline là một chuỗi con so khớp `IndexOf` không phân biệt hoa thường trên `FullName` (`PipelineTestRunner.ShouldIncludeTest`), không phải cú pháp danh sách của NUnit. Filter đơn `QueueProbeATests` → 2 test; `QueueProbeATests;QueueProbeBTests` (cả tên đầy đủ) → 0 test. Vì vậy `queueMergeFilters = false`: mỗi filter một `run_tests`, nối tiếp dưới cùng một refresh — vẫn tiết kiệm compile + reload là phần chi phối.
- `utk editor status` không có `isPlaying`; trường thật là `"playMode":"stopped"|"playing"` → `waitPlaying` tìm `"playMode":"playing"`.
- `utk --raw run_tests` trả về ngay ("running") vì `--async_tests`; coordinator gọi `UTK_RAW_REPORT=1 utk run_tests` để nhận envelope đầy đủ sau khi poll.
- **Bước 3b trên DU04 — xong (2026-10-05):** `m_EnterPlayModeOptionsEnabled: 1`, `m_EnterPlayModeOptions: 1` (chỉ tắt Reload Domain). Rà static: grep của plan cho 358 hit (30 code dự án) nhưng bỏ sót ~45 singleton `instance` không có initializer → quét lại bằng regex không cần `=`. Reset gom trong một file `Assets/Scripts/PlayModeStaticReset.cs` (`[RuntimeInitializeOnLoadMethod(SubsystemRegistration)]`): null 45 singleton, `EventMenuThumb.failedThumbEventIds.Clear()`, `Ads.OnRewardLoaded = null`, `IAPController.m_StoreController/m_IsInitialized/m_IsInitializing` (reflection), chuỗi `LanguageData` về `""`. Bỏ qua có chủ đích: PlayerPrefsX scratch, chuỗi key PlayerPrefs, third-party. Kiểm: 5 shot MainMenu + 2 GamePlay liên tiếp đều PASS, 7 s/shot (Play entry 4 s so với 5 s trước); console chỉ có 3 NRE có sẵn từ baseline (`RandomModel.Start/OnDisable`, `GameController.Start`) do mở thẳng GamePlay không qua LoadingScreen — không phải do reload. Ảnh GamePlay giống hệt baseline; MainMenu khác do state (popup Daily Mission, danh sách event).
- **Bước 3b trên DU02 — xong (2026-10-05):** cùng cách DU04 (pipeline nâng 0.5.0 → 0.8.0-exp.1 khi Editor đóng, backup `/tmp/DU02-manifest.backup.json`). `Assets/Scripts/PlayModeStaticReset.cs` null 59 singleton (gồm `AddressablesManagement.AddressablesManager`, `LocalizationCoroutineRunner`, `OnboardingManager`, `MenuPopupQueue`), reset `Ads.OnRewardLoaded`, `IconMoreGames.OnClickIcon`, event `AvatarElement.OnAvatarSelected` / `LocalizationManager.OnLanguageChanged` (reflection backing field), `FBAnalytics.initAttemptDone`, `EventModeTracking.iconShownThisSession`, `LoadingTracker._stuck*`. Kiểm: 5 shot MainMenu + 2 GamePlay PASS, Play entry 8 s → 4 s; console: **không có site lỗi mới** so với baseline (18 lỗi có sẵn do mở scene không qua LoadingScreen; chỉ `Ads.ChangeSafeAreaWithBannerHeight:325` tăng 1→2 lần qua 7 phiên). Ảnh GamePlay giống hệt; MainMenu lần 5 hiện tooltip tutorial "Model" (state game, không phải leak static — static tồn dư sẽ *ẩn* tutorial chứ không hiện).
- `utk quit` của pipeline 0.8.0 lỗi (`DontDestroyOnLoad` trong editor script) → đóng Editor bằng `utk exec --code 'UnityEditor.EditorApplication.Exit(0); return "";'` (trả "Invalid response format" nhưng Editor thoát).
- Chưa kiểm được `utk status` khi Editor đang kẹt modal dialog (không tạo được dialog từ CLI); giả định "thoát ≠ 0 / timeout → EDITOR_BLOCKED" vẫn là giả định.

## 12. Thứ tự thực hiện

1. Bước 0–1: `unity-job.sh --kind/--gate` + telemetry + `utk queue stats` + cập nhật skill/agents-block.
2. Bước 2: `utk queue serve|submit|status` với `compile` và `test` (gộp, tách, gate, lock, telemetry).
3. Bước 2b: `scene` (SJF, timeout, process group).
4. Bước 3a: `shot` (Play batch, LoadScene giữa script, isolated, edit).
5. Skill mới `utk-editor-queue` + sửa `utk-cli-core`, `utk-test-runner`, `utk-playmode-driving`, agents-block: khi nào gọi thẳng, khi nào `submit`.
6. Bước 3b: Enter Play Mode Options ở DU04 rồi DU02 (ngoài repo).
7. Đo baseline §11, quyết định có cần fairness/ưu tiên hay không.
