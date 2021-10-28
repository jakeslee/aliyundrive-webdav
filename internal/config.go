package internal

const Version = "0.0.8"

type config struct {
	Host         string `arg:"-h" help:"监听地址" default:"0.0.0.0"`
	Port         int    `arg:"-p" help:"监听端口" default:"18080"`
	RefreshToken string `arg:"-r,env:REFRESH_TOKEN" help:"Refresh Token" default:"false"`
	RapidUpload  bool   `arg:"--rapid,env:RAPID" help:"秒传，默认关闭" default:"false"`
	WorkDir      string `arg:"-w,env:WORK_DIR" help:"工作目录，用于保存 RefreshToken 刷新结果" default:"/tmp"`
	UploadSpeed  int    `arg:"--upload-speed,env:UPLOAD_SPEED" help:"上传速度限制，单位 MB/s，默认无限制"`
	AuthType     string `arg:"-a,env:AUTH_TYPE" help:"认证类型，可选 basic, none" default:"none"`
	HttpUsername string `arg:"--http-user,env:HTTP_USER" help:"Basic Auth：登录帐号"`
	HttpPassword string `arg:"--http-pass,env:HTTP_PASS" help:"Basic Auth：登录密码"`
}

func (c *config) Version() string {
	return "aliyundrive-webdav " + Version
}

var Config = &config{}
