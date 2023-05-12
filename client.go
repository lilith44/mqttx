package mqttx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/golang/protobuf/proto"
	"go.uber.org/zap"
)

type Client struct {
	client mqtt.Client

	publishTimeout time.Duration
	logger         *zap.SugaredLogger
	subscribes     map[string]SubscribeOptions
	mutex          sync.Mutex
}

func New(config Config, logger *zap.SugaredLogger, clientIdGenerator func(string) string) (*Client, error) {
	client := &Client{
		publishTimeout: config.Broker.PublishTimeout * time.Second,
		logger:         logger,
		subscribes:     make(map[string]SubscribeOptions),
	}

	options := mqtt.NewClientOptions()
	options.AddBroker(config.Broker.Address)
	options.SetUsername(config.Broker.Username)
	options.SetPassword(config.Broker.Password)
	options.SetClientID(clientIdGenerator(config.Broker.ClientId))
	options.SetKeepAlive(config.Broker.KeepAliveInterval * time.Second)

	options.OnConnectionLost = func(c mqtt.Client, err error) {
		logger.Errorf("与broker断开连接：%s，尝试重连", err)
	}

	options.OnConnect = func(c mqtt.Client) {
		logger.Infof("连接broker成功")

		for _, subscribe := range client.subscribes {
			client.Subscribe(&subscribe)
		}
	}

	client.client = mqtt.NewClient(options)
	if token := client.client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return client, nil
}

type PublishOptions struct {
	// 发布主题
	Topic string
	// 消息质量 0-至多发送一次 1-保证消费至少一次 2-保证只消费一次
	Qos byte
	// 消费后是否保存在broker上
	Retained bool
	// 消息内容，可以是proto.Message或者string, []byte, bytes.Buffer，其余类型会进行json序列化
	Payload any
	// 延迟发送时间，单位秒
	Delayed time.Duration
}

func (c *Client) Publish(options *PublishOptions) error {
	var (
		err  error
		data any
	)

	switch p := options.Payload.(type) {
	case proto.Message:
		if data, err = proto.Marshal(p); err != nil {
			return err
		}

	case []byte, string, bytes.Buffer:
		data = p

	default:
		if data, err = json.Marshal(p); err != nil {
			return err
		}
	}

	topic := getDelayedTopic(options.Topic, options.Delayed)
	token := c.client.Publish(topic, options.Qos, options.Retained, data)

	c.logger.Infof("发布消息至%s，payload：%s", topic, data)

	if !token.WaitTimeout(c.publishTimeout) {
		c.logger.Errorf("发布消息至%s超时", topic)

		return nil
	}

	if err = token.Error(); err != nil {
		c.logger.Errorf("发布消息至%s发生错误：%s", topic, err)

		return err
	}

	return nil
}

type SubscribeOptions struct {
	// 订阅主题
	Topic string
	// 是否共享订阅
	Share bool
	// 订阅用户组
	Group *string
	// 消息质量
	Qos byte
	// 回调函数
	Callback mqtt.MessageHandler
}

func (c *Client) Subscribe(options *SubscribeOptions) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	topic := options.Topic
	if options.Share {
		topic = getSharedTopic(topic, options.Group)
	}

	c.logger.Infof("开始订阅主题%s", topic)

	c.subscribes[topic] = SubscribeOptions{
		Topic:    topic,
		Share:    options.Share,
		Group:    options.Group,
		Qos:      options.Qos,
		Callback: options.Callback,
	}

	c.client.Subscribe(topic, options.Qos, options.Callback)

	c.logger.Infof("订阅主题%s成功", topic)
}

type UnsubscribeOptions struct {
	// 订阅主题
	Topic string
	// 是否共享订阅
	Share bool
	// 订阅用户组
	Group *string
}

func (c *Client) Unsubscribe(options ...*UnsubscribeOptions) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	topics := make([]string, 0, len(options))
	for _, option := range options {
		topic := option.Topic
		if option.Share {
			topic = getSharedTopic(topic, option.Group)
		}

		if _, ok := c.subscribes[topic]; !ok {
			continue
		}

		topics = append(topics, topic)
	}

	c.logger.Infof("取消订阅主题：%v", topics)

	c.client.Unsubscribe(topics...)
	for _, topic := range topics {
		delete(c.subscribes, topic)
	}
}

func getDelayedTopic(topic string, delayed time.Duration) string {
	if delayed != 0 && !strings.HasPrefix(topic, "$delayed") {
		topic = fmt.Sprintf("$delayed/%d/%s", delayed, topic)
	}

	return topic
}

func getSharedTopic(topic string, group *string) string {
	if group != nil {
		return fmt.Sprintf("$share/%s/%s", *group, topic)
	}

	return fmt.Sprintf("$queue/%s", topic)
}
