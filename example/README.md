#

### Build
```shell
go build -o gossipd main.go
```

### Run

#### Launch first node
```shell
$ ./gossipd -addr 127.0.0.1:9090
```

#### Launch 2 more nodes
```shell
$ ./gossipd -addr 127.0.0.1:9091 -peers 127.0.0.1:9090
```

```shell
$ ./gossipd -addr 127.0.0.1:9092 -peers 127.0.0.1:9090
```

#### Query the HTTP api
```shell
$ curl -v localhost:9090/peers
```

```shell
$ curl -v localhost:9090/local
```
