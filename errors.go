package inlet_http

import (
	"github.com/gogap/errors"
)

var INLET_HTTP_ERR_NS = "INLET_HTTP"

var (
	ERR_MESSAGE_ID_IS_EMPTY    = errors.TN(INLET_HTTP_ERR_NS, 1, "message id is empty")
	ERR_PAYLOAD_CHAN_NOT_EXIST = errors.TN(INLET_HTTP_ERR_NS, 2, "payload chan not exist, message id: {{.id}}")
	ERR_ERROR_CHAN_NOT_EXIST   = errors.TN(INLET_HTTP_ERR_NS, 3, "error chan not exist, message id: {{.id}}")

	ERR_READ_HTTP_BODY_FAILED      = errors.TN(INLET_HTTP_ERR_NS, 4, "read http body failed,error: {{.err}}")
	ERR_UNMARSHAL_HTTP_BODY_FAILED = errors.TN(INLET_HTTP_ERR_NS, 5, "unmarshal http body failed, error: {{.err}}")

	ERR_PARSE_COMMAND_TO_OBJECT_FAILED = errors.TN(INLET_HTTP_ERR_NS, 6, "parse command {{.cmd}} error, error: {{.err}}")
	ERR_REQUEST_TIMEOUT                = errors.TN(INLET_HTTP_ERR_NS, 7, "request timeout, graph: {{.graphName}}, message id: {{.msgId}}")
	ERR_GATHER_RESPONSE_TIMEOUT        = errors.TN(INLET_HTTP_ERR_NS, 8, "gather response timeout, graph: {{.graphName}}")

	ERR_REQUEST_TIMEOUT_VALUE_FORMAT_WRONG = errors.TN(INLET_HTTP_ERR_NS, 9, "request timeout value format wrong, value: {{.value}}")

	ERR_MARSHAL_STAT_DATA_FAILED = errors.TN(INLET_HTTP_ERR_NS, 10, "marshal stat data failed, err: {{.err}}")
)
