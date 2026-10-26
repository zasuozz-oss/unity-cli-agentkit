// api_probe.cs — GET one endpoint with the Editor's logged-in token; the token never leaves the Editor.
// Copy to AgentScripts/, fill the three values, run: utk exec --file AgentScripts/api_probe.cs
// Output: "<status> <bytes> bytes -> <file> token.len=<n>". Delete /tmp/api_probe when done.
string BASE = "https://example.com";                  // from the game's API client (grep "Bearer")
string PATH = "/api/user/profile?user_id=<id>";       // GET only
string token = PlayerPrefs.GetString("USER_TOKEN");   // the game's token source; "" sends no Authorization

string name = System.Text.RegularExpressions.Regex.Replace(PATH.Split('?')[0].Trim('/'), "[^A-Za-z0-9]+", "_");
string outFile = "/tmp/api_probe/" + name + ".json";
var http = new System.Net.Http.HttpClient { Timeout = System.TimeSpan.FromSeconds(20) };
try
{
    var req = new System.Net.Http.HttpRequestMessage(System.Net.Http.HttpMethod.Get, BASE + PATH);
    if (!string.IsNullOrEmpty(token))
        req.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", token);
    var res = http.SendAsync(req).Result;
    string body = res.Content.ReadAsStringAsync().Result;
    System.IO.Directory.CreateDirectory("/tmp/api_probe");
    System.IO.File.WriteAllText(outFile, body);
    return $"{(int)res.StatusCode} {body.Length} bytes -> {outFile} token.len={token?.Length ?? 0}";
}
finally { http.Dispose(); }
