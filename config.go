package mqttx

import "time"

type Config struct {
	Broker BrokerConfig `yaml:"broker"`
	Client ClientConfig `yaml:"client"`
}

type BrokerConfig struct {
	// broker地址
	Address string `yaml:"address" validate:"required"`
	// 服务内部连接broker的用户名
	Username string `yaml:"username"`
	// 服务内部连接broker的密码
	Password string `yaml:"password"`
	// 服务内部连接broker的客户端id
	ClientId string `yaml:"clientId" validate:"required"`
	// 服务内部连接broker的保活时间
	KeepAliveInterval time.Duration `yaml:"keepAliveInterval" validate:"min=1"`
	// 发布消息的超时时间，单位秒
	PublishTimeout time.Duration `yaml:"publishTimeout" validate:"min=1"`
}

type ClientConfig struct {
	// 用于外部客户端连接的tcp地址
	TCPAddress string `yaml:"tcpAddress" validate:"required"`
	// 用于外部客户端连接的ws地址
	WSAddress string `yaml:"wsAddress" validate:"required"`
}
