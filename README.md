# aliyundrive-webdav
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fjakeslee%2Faliyundrive-webdav.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fjakeslee%2Faliyundrive-webdav?ref=badge_shield)


基于 aliyundrive 开发的 WebDAV 服务。

### 配置

```shell
$ ./aliyundrive-webdav -h        
aliyundrive-webdav 0.0.7
Usage: aliyundrive-webdav [--host HOST] [--port PORT] [--refreshtoken REFRESHTOKEN] [--rapid] [--workdir WORKDIR] [--upload-speed UPLOAD-SPEED]

Options:
  --host HOST, -h HOST   监听地址 [default: 0.0.0.0]
  --port PORT, -p PORT   监听端口 [default: 18080]
  --refreshtoken REFRESHTOKEN, -r REFRESHTOKEN
                         Refresh Token [default: false, env: REFRESH_TOKEN]
  --rapid                秒传，默认关闭 [default: false, env: RAPID]
  --workdir WORKDIR, -w WORKDIR
                         工作目录，用于保存 RefreshToken 刷新结果 [default: /tmp, env: WORK_DIR]
  --upload-speed UPLOAD-SPEED
                         上传速度限制，单位 MB/s，默认无限制 [env: UPLOAD_SPEED]
  --help, -h             display this help and exit
  --version              display version and exit
```

### 秒传模式说明

由于 WebDAV 的限制，并不能原生支持秒传上传。组件通过在服务器中缓存文件，再使用秒传上传的方式实现秒传到阿里云盘。

对于不能秒传的文件，由于文件需要先上传到组件运行环境，再上传到阿里云盘服务器，文件真正上传到阿里云盘时间可能会比预期的时间要长，所以本模式**默认关闭**。

同时由于需要服务器中转，组件的运行环境的磁盘空间应在 50GB 以上，防止被缓存文件占满。文件上传完后会自动删除缓存文件。

为了优化秒传模式，上传到服务器后中转到阿里云盘时文件不可访问的问题，请求时会回退到本地缓存的文件作为响应。成功上传后才使用阿里云盘的文件作为响应。



## License
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fjakeslee%2Faliyundrive-webdav.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fjakeslee%2Faliyundrive-webdav?ref=badge_large)