---
name: release-production
description: Use when preparing, cutting, or tagging a production/release build of a Unity game (ra bản production, build bản thật, release store, tăng version), when switching dev ↔ prod config, when setting up app-config-checklist.md for a game, or when worried that ad keys (IronSource/LevelPlay, AdMob), Firebase google-services.json / GoogleService-Info.plist, or the package/bundle id are mixed between test and production or between Android and iOS.
---

# Release production (mọi game Unity)

## Nguyên tắc

- Branch dev (mặc định `main`) **luôn mang mã dev**, branch prod (mặc định `production`) **luôn mang mã thật**. Chỉ merge dev → prod, **không bao giờ** merge ngược.
- **Mọi commit code/fix đều vào `$DEV`** (hoặc branch tính năng đang đứng, miễn không phải `$PROD`), rồi push `$DEV`. `$PROD` = `$DEV` + **đúng một commit release** chỉ chứa mã sản phẩm và version. Không bao giờ commit fix trên `$PROD`; fix cho bản release → commit trên `$DEV`, push, rồi release lại.
- Mọi thứ riêng của từng game nằm trong `app-config-checklist.md` ở root project: bảng mã (`<!-- release-config:table -->`) và settings (`<!-- release-config:settings -->`: `dev_branch`, `prod_branch`, `tag`). Skill không chứa giá trị hay đường dẫn cố định nào.
- **Phần viết tay trong checklist là của user**, không bao giờ xoá hay viết lại: mọi thứ nằm ngoài hai block có marker (vd mục "✅ Kiểm tra thủ công" chứa mã thật để user so bằng mắt, ghi chú, tham khảo). Chỉ sửa dòng trong bảng/settings; sau khi sửa, `git diff app-config-checklist.md` chỉ được đổi đúng các dòng đó. Thêm SDK/key mới → thêm vào bảng **và** vào mục kiểm tra thủ công nếu có.
- Ghi/kiểm tra mã **chỉ qua script**, không sửa tay: `S=.claude/skills/release-production/scripts/release_config.py` (chạy ở root project; `python3 $S --help` có mô tả định dạng bảng).
- `check` fail ở bất kỳ bước nào → **dừng, báo nguyên văn cho user**, không commit/push.
- Đọc tên branch từ checklist: `DEV=$(python3 $S setting dev_branch)`, `PROD=$(python3 $S setting prod_branch)`, `TAG=$(python3 $S setting tag)`.

## Setup lần đầu cho một game (chưa có bảng)

1. Đứng trên branch dev, `python3 $S init`. Script dò các file SDK chuẩn (ProjectSettings, LevelPlay, AdMob, Firebase) và điền cột Dev bằng giá trị hiện tại.
2. Tìm mã **nằm ngoài vị trí chuẩn** (key hardcode trong C#, config riêng): grep các file có key hiện tại hoặc các từ `app_key`, `AppKey`, `adUnit`, `BANNER`, `INTERSTITIAL`, `REWARDED`. Thêm mỗi chỗ một dòng `regex`; pattern phải bắt được **mọi** dòng gán không comment (vd `^\s*public static string X = "([^"]*)";`). Mã trong code giữ **cả hai block** có nhãn (`//Test` rồi `//Release`, mỗi dòng của block đang tắt comment bằng `// `): `apply` comment dòng đang chạy và bỏ comment dòng mang giá trị env, không sửa giá trị — nhìn code là biết block nào test, block nào release. Chỉ khi không dòng nào mang giá trị env, script mới thay giá trị tại chỗ.
   File config Firebase: giữ cả hai bản cạnh file đang dùng (`Assets/google-services-test.json`, `Assets/google-services-product.json`); cột Dev/Prod của dòng `file` trỏ vào hai bản này, `apply` copy bản của env vào `google-services.json`.
   Game **có dùng** plugin nào thì thêm cả **file setup của plugin đó trong Editor** (vd `ProjectSettings/GooglePlayGameSettings.txt`: `proj.AppId`, `and.ClientId`, `android.SetupDone`) dưới dạng `verify` — không chỉ file runtime (`GameInfo.cs`, manifest). Máy build đọc file setup này; runtime đúng không có nghĩa là setup còn. Giá trị dùng chung cho cả hai env (Dev = Prod) → file đó chỉ kiểm tra có mặt, không bị quét leak.
3. Hỏi user từng giá trị `?` (cột Prod, và đường dẫn file nguồn của các dòng `file`). File nguồn dev/prod phải được lưu riêng trong repo (vd `dev-config-backup/…`, `…-product.json`).
4. Checklist quyết định kiểm tra gì: game không dùng plugin/platform nào → không thêm dòng, hoặc để `—` (file không có vẫn OK). Có giá trị mà file mất → `check` fail. Platform chưa cấu hình → để `—`. Key đặt tên theo `android.*` / `ios.*` để script chặn trùng mã giữa hai platform.
5. `python3 $S check dev` phải OK trên branch dev. Nó fail → sửa bảng, không sửa code.

## Quy trình release

1. **Tiền kiểm** (trên `$DEV`):
   - Đang đứng trên `$PROD` với thay đổi code → `git stash` → `git checkout $DEV` → `git stash pop`, không commit trên `$PROD`.
   - `git status --porcelain` rỗng. Có thay đổi → hỏi user commit hay stash, không tự quyết. Commit thì commit **trên `$DEV`**.
   - **File máy build tự sinh không bao giờ vào commit**, kể cả khi user bảo "commit tất cả": `Assets/Plugins/Android/mainTemplate.gradle`, `settingsTemplate.gradle`, `gradleTemplate.properties` (khối `// Android Resolver … Start/End`), `ProjectSettings/AndroidResolverDependencies.xml`, `Assets/Plugins/Android/FirebaseCrashlytics.androidlib/res/values/crashlytics_build_id.xml`. Mỗi máy resolve/build lại các file này nên chúng luôn bị sửa cục bộ ở máy build. Commit bản của máy mình thì máy build `git pull` sẽ abort (`Your local changes … would be overwritten by merge`). Trước `git add -A`: `git checkout -- <các file đó>` (hoặc `git restore --staged`). Nếu đã lỡ push: lấy lại bản cũ (`git checkout <commit trước> -- <file>`), commit trên `$DEV`, merge sang `$PROD`, rồi kiểm tra `git diff --shortstat <prod cũ> origin/$PROD -- <file>` ra rỗng.
   - `git push origin $DEV` (có commit chưa push thì push trước khi merge), để mọi thứ `$PROD` nhận đều đã có trên `origin/$DEV`.
   - `utk status` reachable. `git fetch origin --tags`.
   - `python3 $S check dev` phải OK.
2. **Version:** `python3 $S next-version` → hỏi user xác nhận version name (mặc định tăng patch) **và** version code (mặc định +1, user có thể nhập tay). Không tự chọn khi user chưa xác nhận.
3. **Sang prod:**
   - Chưa có local `$PROD`: có `origin/$PROD` → `git checkout -b $PROD origin/$PROD`, không có → `git checkout -b $PROD $DEV`. Có rồi → `git checkout $PROD` và `git pull --ff-only origin $PROD` (bỏ qua nếu remote chưa có).
   - `git merge --no-edit $DEV`. Conflict → dừng, báo user.
4. **Ghi mã prod:** `python3 $S apply prod` → `python3 $S set-version <name> <code>` → `utk editor refresh` → `python3 $S tidy` (báo restored → `utk editor refresh` lần nữa).
5. **Xác minh:**
   - `python3 $S check prod` phải OK.
   - `utk exec 'return PlayerSettings.GetApplicationIdentifier(UnityEditor.Build.NamedBuildTarget.Android) + " " + PlayerSettings.bundleVersion + " " + PlayerSettings.Android.bundleVersionCode;'` phải khớp package prod + version vừa đặt (Editor đã nạp, không chỉ file).
   - `utk console --type error` sạch.
6. **Commit + tag + push:** `git status --porcelain` chỉ được chứa file thuộc bảng mã (target của các dòng + `ProjectSettings/ProjectSettings.asset`); có file khác → dừng, báo user (đó là code, phải vào `$DEV`). → `git add -A` → `git commit -m "release: v<name> (<code>)"` → `git tag <TAG với {name},{code} đã thay>` → `git push origin $PROD` → `git push origin <tag>`.
7. **Về dev:** `git checkout $DEV` → `utk editor refresh` → `python3 $S tidy` → `python3 $S check dev` phải OK, `utk exec` ở bước 5 trả package dev, `git status --porcelain` rỗng.
8. **Báo cáo:** version, tag, commit hash, kết quả `check prod` / `check dev`. Build không nằm trong skill — nhắc user build từ `$PROD` (hoặc `utk build` nếu user yêu cầu).

## Script kiểm tra những gì

| Kiểm tra | Bắt được |
|---|---|
| Mỗi dòng `regex`/`verify`: **mọi** match = giá trị env | mã test/prod sai chỗ, 2 dòng gán cùng lúc (bỏ comment nhầm) |
| Dòng `file`: giống hệt file nguồn của env; `—` → không được tồn tại | google-services.json / plist nhầm project, plist iOS lọt vào game chưa làm iOS |
| Leak: giá trị chỉ thuộc env kia không xuất hiện trong bất kỳ target nào (bỏ qua dòng comment) | package/project_id dev còn sót trong file prod |
| Platform: `android.X` ≠ `ios.X`, và không nằm trong file riêng của platform kia | dán mã Android vào chỗ iOS và ngược lại |
| `verify` rows (file editor tự sinh) | quên refresh → `google-services.xml` vẫn trỏ project cũ |

## Lỗi hay gặp

- **Sửa mã tay trên branch dev rồi quên revert** → `check dev` ở bước 1 chặn lại.
- **Commit fix trên `$PROD`** hoặc quên push `$DEV` → `origin/$DEV` thiếu commit mà `$PROD` có. Sửa: cherry-pick sang `$DEV`, push. Kiểm tra: `git log origin/$DEV..origin/$PROD --oneline` chỉ được ra commit `release: …`.
- **Block `//Test` đang chạy mã thật** (script cũ ghi đè giá trị thay vì bật/tắt comment, từng gặp ở `GameConfigs.cs`). Sau `apply prod`, `git diff` phải cho thấy block `//Test` bị comment và block `//Release` được bỏ comment. Sửa: lấy lại file từ `$DEV` (`git checkout $DEV -- <file>`) rồi `apply prod`.
- **Commit gộp (`git add -A`, "sync working tree") xoá mất setup plugin** → Editor ở máy chưa setup ghi lại file settings chỉ còn vài dòng mặc định (từng mất 2 lần ở `GooglePlayGameSettings.txt`). Các dòng `verify` trên file setup chặn ở `check`; sửa bằng cách lấy lại nội dung từ commit cũ hoặc checklist, không bấm Setup trên `$DEV` (nó đòi đổi package sang prod).
- **Commit gộp kéo theo output Android Resolver / Crashlytics build id** → máy build pull lỗi "would be overwritten by merge" ở `mainTemplate.gradle`, `settingsTemplate.gradle`, `AndroidResolverDependencies.xml`, `crashlytics_build_id.xml`. Xem bước 1: loại các file đó trước khi commit; đã push rồi thì revert về bản cũ, không bảo máy build stash mãi.
- **Merge prod → dev** → mã thật lọt vào dev. Nếu lỡ: revert commit merge, `apply dev` + `check dev`.
- **Mã mới thêm vào code mà không thêm vào bảng** → script không biết để kiểm tra. Thêm SDK/key mới = thêm dòng vào bảng.
- **Plugin sinh lại file làm đổi GUID trong `.meta`** → luôn chạy `tidy` sau refresh.
