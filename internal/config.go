package internal

type config struct {
	Host         string `arg:"-h" help:"监听地址" default:"0.0.0.0"`
	Port         int    `arg:"-p" help:"监听端口" default:"18080"`
	RefreshToken string `arg:"-r,required,env:REFRESH_TOKEN" help:"Refresh Token" default:""`
	RapidUpload  bool   `arg:"--rapid,env:RAPID" help:"秒传，默认关闭" default:"false"`
}

var Config = &config{}
