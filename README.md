# aliyundrive-webdav

基于 aliyundrive 开发的 WebDAV 服务。

### 配置

```shell
$ ./aliyundrive-webdav -h        
aliyundrive-webdav 0.0.1
Usage: main [--host HOST] [--port PORT] --refreshtoken REFRESHTOKEN [--rapid]

Options:
  --host HOST, -h HOST   监听地址 [default: 0.0.0.0]
  --port PORT, -p PORT   监听端口 [default: 18080]
  --refreshtoken REFRESHTOKEN, -r REFRESHTOKEN
                         Refresh Token [env: REFRESH_TOKEN]
  --rapid                秒传，默认关闭 [default: false, env: RAPID]
  --help, -h             display this help and exit
  --version              display version and exit
```
