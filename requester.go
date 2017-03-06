package inlet_http

import (
	"time"

	"github.com/gogap/spirit"
)

type Requester interface {
	Request(graph spirit.MessageGraph, payload spirit.Payload, payloadRespChan chan spirit.Payload, errResp chan error) (msgId string, err error)
	OnMessageReceived(payload spirit.Payload)
	OnMessageError(payload spirit.Payload)
	OnMessageProcessed(messageId string)
	SetTimeout(timeout time.Duration)
	GetTimeout() time.Duration
}
