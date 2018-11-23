package flex

type Status string

const (
	StatusSuccess      Status = "Success"
	StatusFailure      Status = "Failure"
	StatusNotSupported Status = "Not Supported"
)

type Response struct {
	Status  Status `json:"status"`
	Message string `json:"message"`
	Device  string `json:"device,omitempty"`

	Capabilities Capabilities `json:"capabilities,omitempty"`
}

func Failure(msg string) Response {
	return Response{
		Status:  StatusFailure,
		Message: msg,
	}
}

func Success(msg string) Response {
	return Response{
		Status:  StatusSuccess,
		Message: msg,
	}
}

func NotSupported(msg string) Response {
	return Response{
		Status:  StatusNotSupported,
		Message: msg,
	}
}

type Capabilities struct {
	Attach bool `json:"attach"`
}
