#!/usr/bin/env python3
"""Adapt ONLY the historical tagged synthetic backend to an owned loopback server.
No host, worker, store, launcher or shutdown implementation is modified.
"""
import pathlib
import sys
p = pathlib.Path(sys.argv[1]) / 'source/server/cmd/cercano/development_provider_fixture.go'
s = p.read_text()
s = s.replace('"context"', '"context"\n "bytes"\n "net/http"\n "net/url"')
needle = 'func (*acceptanceProvider) Chat(ctx context.Context, req inference.Call) (inference.Result, error) {'
assert s.count(needle) == 1
s = s.replace(needle, needle + '''
 endpoint, err := os.ReadFile(filepath.Join(os.Getenv("CERCANO_FIXTURE_BARRIER"), "loopback"))
 if err != nil { return inference.Result{}, err }
 u, err := url.Parse(string(endpoint))
 if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" { return inference.Result{}, fmt.Errorf("private loopback required") }
 body, err := json.Marshal(req)
 if err != nil { return inference.Result{}, err }
 request, err := http.NewRequestWithContext(ctx, "POST", string(endpoint), bytes.NewReader(body))
 if err != nil { return inference.Result{}, err }
 client := &http.Client{Transport: &http.Transport{Proxy:nil}, CheckRedirect:func(*http.Request,[]*http.Request)error{return fmt.Errorf("redirect refused")}}
 defer client.CloseIdleConnections()
 response, err := client.Do(request)
 if err != nil { return inference.Result{}, err }
 defer response.Body.Close()
 var result inference.Result
 err = json.NewDecoder(response.Body).Decode(&result)
 return result, err
}

func historicalUnusedSyntheticChat(ctx context.Context, req inference.Call) (inference.Result, error) {
''')
p.write_text(s)
