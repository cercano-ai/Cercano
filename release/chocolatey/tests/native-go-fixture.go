// Synthetic stdlib-only native PE probe. Not product code.
package main

import (
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "time"
)

var version = "unset"
func main() {
 exe, err := os.Executable(); if err != nil { panic(err) }
 exe, err = filepath.Abs(exe); if err != nil { panic(err) }
 resource, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "resource.txt")); if err != nil { panic(err) }
 fmt.Printf("native-go-fixture version=%s resource=%s\n", version, resource)
 if len(os.Args)==1 { return }
 if len(os.Args)!=3 || os.Args[1]!="--hold" { os.Exit(2) }
 control := os.Args[2]
 b, err := json.Marshal(map[string]interface{}{"pid":os.Getpid(),"executable":exe,"version":version,"resource":string(resource),"deadline_seconds":300}); if err != nil { panic(err) }
 if err=os.WriteFile(control+".ready",b,0600); err!=nil { panic(err) }
 until:=time.Now().Add(300*time.Second)
 for time.Now().Before(until) { if _,err=os.Stat(control+".stop"); err==nil { fmt.Println("cooperative-stop"); return }; time.Sleep(100*time.Millisecond) }
 fmt.Println("finite-lifetime-expired")
}
