package inlet_http

import (
	"github.com/gogap/spirit"
)

type Requester interface {
	Init(addr spirit.MessageAddress)
	Request(addrs []spirit.MessageAddress, payload spirit.Payload, response chan spirit.Payload) (msgId string, err error)
	OnMessageReceived(payload spirit.Payload) (result interface{}, err error)
	OnMessageProcessed(messageId string)
	SetMessageSenderFactory(factory spirit.MessageSenderFactory)
	GetMessageSenderFactory() spirit.MessageSenderFactory
}
