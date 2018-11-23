package flex

// -- These structs implement the flex protocol JSON format

// Status define the possible flex cmd output statuses
type Status string

// All possible statuses are listed here
const (
	StatusSuccess      Status = "Success"
	StatusFailure      Status = "Failure"
	StatusNotSupported Status = "Not Supported"
)

// Response to return as JSON to the kubelet
type Response struct {
	Status       Status       `json:"status"`
	Message      string       `json:"message"`
	Device       string       `json:"device,omitempty"`
	Capabilities Capabilities `json:"capabilities,omitempty"`
}

// Capabilities of the flex driver implementation
// (eg. does it support attach/mountdevice/etc. or only mount/unmout?)
type Capabilities struct {
	Attach bool `json:"attach"`
}

// -- Helper methods for flex responses

// Failure returns a StatusFailure with the given msg
func Failure(msg string) Response {
	return Response{
		Status:  StatusFailure,
		Message: msg,
	}
}

// Success returns a StatusSuccess with the given msg
func Success(msg string) Response {
	return Response{
		Status:  StatusSuccess,
		Message: msg,
	}
}

// NotSupported returns a StatusNotSupported with the given msg
func NotSupported(msg string) Response {
	return Response{
		Status:  StatusNotSupported,
		Message: msg,
	}
}
