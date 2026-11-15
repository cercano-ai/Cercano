# Inert open-only probes: no real-state bytes read, no real-state writes, no credentials queried.
require 'socket'
r='/opt/cercano-migration-unpriv-20261115'
puts "uid=#{Process.uid} gid=#{Process.gid} groups=#{Process.groups.inspect}"
raise 'privileged process group' unless (Process.groups & [0,20,80]).empty?
File.write("#{r}/tmp/allowed-probe", 'INERT')
raise 'positive read/write failed' unless File.read("#{r}/tmp/allowed-probe") == 'INERT'
puts 'PASS owned-prefix read/write'
s=UNIXServer.new("#{r}/tmp/allowed-probe.sock"); s.close
puts 'PASS UNIX-domain bind'
failures=[]
probes={
 'real-home read'=> ->{File.open('/Users/test/.zprofile', 'r').close},
 'real-home write-open'=> ->{File.open('/Users/test/.zprofile', 'r+').close},
 'system-keychain read'=> ->{File.open('/Library/Keychains/System.keychain','r').close},
 'user-keychain read'=> ->{File.open('/Users/test/Library/Keychains/login.keychain-db','r').close},
 'real-brew read'=> ->{File.open('/opt/homebrew/README.md','r').close},
 'real-brew write-open'=> ->{File.open('/opt/homebrew/README.md','r+').close},
 'old-fixture read'=> ->{File.open('/opt/cercano-migration-20261115/safety.sb','r').close},
 'TCP loopback synthetic endpoint'=> ->{Socket.tcp('127.0.0.1',49382,connect_timeout:1).close},
 'signal permission inert self-zero'=> ->{Process.kill(0,Process.pid)}
}
probes.each do |name,fn|
 begin
  fn.call; puts "FAIL unexpectedly allowed #{name}"; failures << name
 rescue Errno::EPERM,Errno::EACCES => e
  puts "PASS denied #{name}: #{e.class}"
 rescue => e
  puts "FAIL inconclusive #{name}: #{e.class}"; failures << name
 end
end
exit(failures.empty? ? 0 : 1)
