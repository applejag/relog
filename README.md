# relog - A log re-formatter CLI

Reads JSON or logfmt input, and outputs with its own colored logging output.

JSON logs are really useful for indexing inside Elasticsearch/Kibana, but
horrible to read for us Kubernetes maintainers that just want to do some quick
`kubectl logs my-pod`

With `relog`, you pipe the log output right through and it figures out the rest.

## Features

- MongoDB logs parsing
- Multiline-JSON logs parsing (such as Elasticsearch stacktraces)
- Parses logfmt logs
- Streaming, meaning support for `kubectl logs -f my-pod | relog`

## Install

Requires Go 1.19 (or higher)

```sh
go install github.com/jilleJr/relog@latest
```

> :warning: Currently depends on the `bytedance/sonic` JSON parsing library
> which has a strong dependency on Amd64 architecture.
>
> In other words, this does not work on ARM
> (such as Mac M1 or some Windows Surface laptops)

## Example

### MongoDB logs

```console
$ kubectl logs mongodb-0
...
{"t":{"$date":"2022-09-20T17:56:28.918+00:00"},"s":"I",  "c":"NETWORK",  "id":22943,   "ctx":"listener","msg":"Connection accepted","attr":{"remote":"127.0.0.1:34990","uuid":"611afe09-86aa-48d4-807f-f906eeda879e","connectionId":23325,"connectionCount":38}}
{"t":{"$date":"2022-09-20T17:56:28.919+00:00"},"s":"I",  "c":"NETWORK",  "id":51800,   "ctx":"conn2651307","msg":"client metadata","attr":{"remote":"127.0.0.1:3906","client":"conn2651307","doc":{"application":{"name":"MongoDB Shell"},"driver":{"name":"MongoDB Internal Client","version":"5.0.10-9"},"os":{"type":"Linux","name":"Ubuntu","architecture":"x86_64","version":"20.04"}}}}
{"t":{"$date":"2022-09-20T17:56:28.928+00:00"},"s":"I",  "c":"NETWORK",  "id":22944,   "ctx":"conn2651307","msg":"Connection ended","attr":{"remote":"127.0.0.1:34976","uuid":"628afe16-78aa-48d4-799f-f906eeda821e","connectionId":2212301,"connectionCount":37}}

$ kubectl logs mongodb-0 | relog
...
Sep-20 19:56 INF [NETWORK|listener   |22943] Connection accepted connectionCount=38 connectionId=23325 remote=127.0.0.1:34990 uuid=611afe09-86aa-48d4-807f-f906eeda879e
Sep-20 19:56 INF [NETWORK|conn2651307|51800] client metadata client=conn2651307 doc={"application":{"name":"MongoDB Shell"},"driver":{"name":"MongoDB Internal Client","version":"5.0.10-9"},"os":{"architecture":"x86_64","name":"Ubuntu","type":"Linux","version":"20.04"}} remote=127.0.0.1:3906
Sep-20 19:56 INF [NETWORK|conn2651307|22944] Connection ended connectionCount=37 connectionId=2212301 remote=127.0.0.1:34976 uuid=628afe16-78aa-48d4-799f-f906eeda821e
```

### Elasticsearch logs

```console
$ kubectl logs elasticsearch-master-0
...
{"type": "server", "timestamp": "2022-08-03T10:37:08,390Z", "level": "INFO", "component": "o.e.x.s.a.TokenService", "cluster.name": "elasticsearch", "node.name": "elasticsearch-master-0", "message": "refresh keys" }
{"type": "server", "timestamp": "2022-08-03T10:37:08,684Z", "level": "INFO", "component": "o.e.x.s.a.TokenService", "cluster.name": "elasticsearch", "node.name": "elasticsearch-master-0", "message": "refreshed keys" }
{"type": "server", "timestamp": "2022-08-03T10:37:08,758Z", "level": "INFO", "component": "o.e.l.LicenseService", "cluster.name": "elasticsearch", "node.name": "elasticsearch-master-0", "message": "license [xxxxxxx-xxxx-xxxx-xxxx-xxxxxxx] mode [basic] - valid" }
{"type": "server", "timestamp": "2022-09-05T16:06:17,918Z", "level": "WARN", "component": "r.suppressed", "cluster.name": "elasticsearch", "node.name": "elasticsearch-master-0", "message": "path: /_snapshot/minio-backups/_all, params: {repository=minio-backups, snapshot=_all}", "cluster.uuid": "loremipsum", "node.id": "foobar" ,
"stacktrace": ["org.elasticsearch.transport.RemoteTransportException: [elasticsearch-master-2][127.0.0.1:9300][cluster:admin/snapshot/get]",
"Caused by: org.elasticsearch.snapshots.SnapshotException: [minio-backups:es-snapshot-01062022-063000-foobar/heyo-mayo] Snapshot could not be read",
"at org.elasticsearch.action.admin.cluster.snapshots.get.TransportGetSnapshotsAction.snapshots(TransportGetSnapshotsAction.java:208) ~[elasticsearch-7.12.1.jar:7.12.1]",
"at org.elasticsearch.action.admin.cluster.snapshots.get.TransportGetSnapshotsAction.lambda$loadSnapshotInfos$1(TransportGetSnapshotsAction.java:159) ~[elasticsearch-7.12.1.jar:7.12.1]",
"at org.elasticsearch.action.ActionRunnable.lambda$supply$0(ActionRunnable.java:47) ~[elasticsearch-7.12.1.jar:7.12.1]",
"at org.elasticsearch.action.ActionRunnable$2.doRun(ActionRunnable.java:62) ~[elasticsearch-7.12.1.jar:7.12.1]",
"at org.elasticsearch.common.util.concurrent.ThreadContext$ContextPreservingAbstractRunnable.doRun(ThreadContext.java:732) ~[elasticsearch-7.12.1.jar:7.12.1]",
"at org.elasticsearch.common.util.concurrent.AbstractRunnable.run(AbstractRunnable.java:26) ~[elasticsearch-7.12.1.jar:7.12.1]",
"at java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1130) ~[?:?]",
"at java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:630) ~[?:?]",
"at java.lang.Thread.run(Thread.java:831) [?:?]" ] }

$ kubectl logs elasticsearch-master-0 | relog
...
Aug-03 12:37 INF refresh keys cluster.name=elasticsearch component=o.e.x.s.a.TokenService node.name=elasticsearch-master-0 type=server
Aug-03 12:37 INF refreshed keys cluster.name=elasticsearch component=o.e.x.s.a.TokenService node.name=elasticsearch-master-0 type=server
Aug-03 12:37 INF license [xxxxxxx-xxxx-xxxx-xxxx-xxxxxxx] mode [basic] - valid cluster.name=elasticsearch component=o.e.l.LicenseService node.name=elasticsearch-master-0 type=server
Sep-05 18:06 WRN path: /_snapshot/minio-backups/_all, params: {repository=minio-backups, snapshot=_all}
	STACKTRACE
	==========
	org.elasticsearch.transport.RemoteTransportException: [elasticsearch-master-2][127.0.0.1:9300][cluster:admin/snapshot/get]
	Caused by: org.elasticsearch.snapshots.SnapshotException: [minio-backups:es-snapshot-01062022-063000-foobar/heyo-mayo] Snapshot could not be read
	at org.elasticsearch.action.admin.cluster.snapshots.get.TransportGetSnapshotsAction.snapshots(TransportGetSnapshotsAction.java:208) ~[elasticsearch-7.12.1.jar:7.12.1]
	at org.elasticsearch.action.admin.cluster.snapshots.get.TransportGetSnapshotsAction.lambda$loadSnapshotInfos$1(TransportGetSnapshotsAction.java:159) ~[elasticsearch-7.12.1.jar:7.12.1]
	at org.elasticsearch.action.ActionRunnable.lambda$supply$0(ActionRunnable.java:47) ~[elasticsearch-7.12.1.jar:7.12.1]
	at org.elasticsearch.action.ActionRunnable$2.doRun(ActionRunnable.java:62) ~[elasticsearch-7.12.1.jar:7.12.1]
	at org.elasticsearch.common.util.concurrent.ThreadContext$ContextPreservingAbstractRunnable.doRun(ThreadContext.java:732) ~[elasticsearch-7.12.1.jar:7.12.1]
	at org.elasticsearch.common.util.concurrent.AbstractRunnable.run(AbstractRunnable.java:26) ~[elasticsearch-7.12.1.jar:7.12.1]
	at java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1130) ~[?:?]
	at java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:630) ~[?:?]
	at java.lang.Thread.run(Thread.java:831) [?:?]
	 cluster.name=elasticsearch cluster.uuid=loremipsum component=r.suppressed node.id=foobar node.name=elasticsearch-master-0 type=server
```

### Prometheus

```console
$ kubectl logs prometheus-server-5bcdc9849-84h8z
...
ts=2022-09-20T17:30:02.249Z caller=compact.go:519 level=info component=tsdb msg="write block" mint=1663689600021 maxt=1663693200000 ulid=xxxxxxxxxxxxxxxxxxxxxxxxxx duration=2.103117406s
ts=2022-09-20T17:30:02.265Z caller=db.go:1292 level=info component=tsdb msg="Deleting obsolete block" block=xxxxxxxxxxxxxxxxxxxxxxxxxx
ts=2022-09-20T17:30:02.358Z caller=head.go:840 level=info component=tsdb msg="Head GC completed" duration=93.435249ms
ts=2022-09-20T17:30:02.366Z caller=checkpoint.go:98 level=info component=tsdb msg="Creating checkpoint" from_segment=1377 to_segment=1378 mint=1663693200000
ts=2022-09-20T17:30:03.550Z caller=head.go:1009 level=info component=tsdb msg="WAL checkpoint complete" first=1377 last=1378 duration=1.184221371s

$ kubectl logs prometheus-server-5bcdc9849-84h8z | relog
...
Sep-20 19:30 INF compact.go:519 > write block component=tsdb duration=2.103117406s maxt=1663693200000 mint=1663689600021 ulid=xxxxxxxxxxxxxxxxxxxxxxxxxx
Sep-20 19:30 INF db.go:1292 > Deleting obsolete block block=xxxxxxxxxxxxxxxxxxxxxxxxxx component=tsdb
Sep-20 19:30 INF head.go:840 > Head GC completed component=tsdb duration=93.435249ms
Sep-20 19:30 INF checkpoint.go:98 > Creating checkpoint component=tsdb from_segment=1377 mint=1663693200000 to_segment=1378
Sep-20 19:30 INF head.go:1009 > WAL checkpoint complete component=tsdb duration=1.184221371s first=1377 last=1378
```
