# Test harness only: reject raw provider graphs, bounded plain-data serialization.
Add-Type -ReferencedAssemblies System.Web.Extensions @'
using System; using System.Threading; using System.Web.Script.Serialization;
public static class BoundedPlainJson {
 public static string Serialize(object value) {
  string result=null; Exception error=null;
  var t=new Thread(()=>{try{var s=new JavaScriptSerializer();s.MaxJsonLength=16777216;result=s.Serialize(value);}catch(Exception e){error=e;}});
  t.IsBackground=true;t.Start();if(!t.Join(5000))throw new TimeoutException("Plain JSON serialization exceeded 5s");
  if(error!=null)throw error;return result;
 }
}
'@
function ConvertTo-PlainJson($Value) {
 $budget=@{nodes=0;chars=0};$clock=[Diagnostics.Stopwatch]::StartNew()
 function Project($v,[int]$level){
  $budget.nodes++;if($level -gt 16 -or $budget.nodes -gt 100000 -or $clock.ElapsedMilliseconds -gt 2000){throw 'Plain projection bound exceeded'}
  if($null -eq $v){return $null}
  if($v -is [string]){$budget.chars+=$v.Length;if($budget.chars -gt 8000000){throw 'Plain string budget exceeded'};return [string]::new($v.ToCharArray())}
  if($v -is [bool] -or $v -is [int] -or $v -is [long] -or $v -is [double]){return $v}
  if($v -is [DateTime]){return $v.ToUniversalTime().ToString('o')}
  if($v -is [Collections.IDictionary]){$d=New-Object 'Collections.Generic.Dictionary[string,object]';foreach($key in $v.Keys){$d.Add([string]$key,(Project $v[$key] ($level+1)))};return ,$d}
  if($v -is [Collections.IList]){$a=New-Object 'Collections.Generic.List[object]';foreach($item in $v){$a.Add((Project $item ($level+1)))};return ,$a}
  if($v -is [pscustomobject]){$d=New-Object 'Collections.Generic.Dictionary[string,object]';foreach($p in $v.PSObject.Properties){if($p.MemberType -ne 'NoteProperty'){throw 'Non-data property rejected'};$d.Add($p.Name,(Project $p.Value ($level+1)))};return ,$d}
  throw ('Raw non-data type rejected: '+$v.GetType().FullName)
 }
 $plain=Project $Value 0
 [BoundedPlainJson]::Serialize($plain)
}
function Read-InstallEvents([string]$Path){
 foreach($line in [IO.File]::ReadAllLines($Path)){
  $e=ConvertFrom-Json -InputObject $line
  [pscustomobject]@{version=[string]$e.version;time=[string]$e.time;pid=[int]$e.pid;exeHash=[string]$e.exeHash;resourceHash=[string]$e.resourceHash}
 }
}
