## Porter
<img src="/docs/logo.png" width="100" height="100"/>
 
### Overview
Porter is a distributed MySQL binlog syncer based on raft. Porter can act as a slave to the real master. 
Porter has the following key featues:
* MySQL replication protocol compatibility,pull the binlog files from the Mysql master through gtid mode.
* High available ,porter uses Raft to support High available,The binlog data written to the porter cluster is guaranteed to be consistent between multiple nodes,
and the order of binlog event is exactly the the same as that on the master

<img src="/docs/architecture.jpg" width="550" height="700"/>

### Requirements
* go version 1.15+
please check you go version
```
go version
```

### Quick Start
1. start porter
```
cd /cmd/porter

go run main.go
```

2. send startSyncer API 
```
curl -H "Content-Type:application/json" -X POST --data '{
    "mysql_addr":"127.0.0.1:3306",
    "mysql_user":"root",
    "mysql_password":"yaoyichen52",
    "mysql_charset":"utf8",
    "mysql_position":"",
    "server_id":1001,
    "flavor":"mysql",
    "mysqldump":"mysqldump",
    "skip_master_data":false,
    "sources": [{
        "schema":"test",
        "tables":["test_river_0000"]
    }]
}' http://localhost:5050/startSyncer
```

### License
```
Copyright (c) 2020 YunLSP+

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### Acknowledgments
* Thanks [misselvexu](https://github.com/misselvexu) give me support and enough free to complete it.
