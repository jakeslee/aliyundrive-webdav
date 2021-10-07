package internal

const version = "0.0.1"

type config struct {
	Host         string `arg:"-h" help:"监听地址" default:"0.0.0.0"`
	Port         int    `arg:"-p" help:"监听端口" default:"18080"`
	RefreshToken string `arg:"-r,env:REFRESH_TOKEN" help:"Refresh Token" default:"false"`
	RapidUpload  bool   `arg:"--rapid,env:RAPID" help:"秒传，默认关闭" default:"false"`
}

func (c *config) Version() string {
	return "aliyundrive-webdav " + version
}

var Config = &config{}
