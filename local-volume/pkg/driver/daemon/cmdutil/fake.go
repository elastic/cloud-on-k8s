package cmdutil

// FakeExecutables Wraps around the FakeExecutable to provide multiple stubs in
// a test.
type FakeExecutables struct {
	// index is incremented on each generation of an Executable.
	index int
	Stubs []*FakeExecutable

	recordedCommands [][]string
}

// NewFakeCmdsBuilder initializes a FakeExecutables and returns a ExecutableFactory
// signature.
func NewFakeCmdsBuilder(stubs []*FakeExecutable) ExecutableFactory {
	var s = &FakeExecutables{Stubs: stubs}
	return s.ExecutableFactory()
}

// ExecutableFactory returns an ExecutableFactory function
func (s *FakeExecutables) ExecutableFactory() ExecutableFactory {
	return func(name string, args ...string) Executable {
		s.index++
		stub := s.Stubs[s.index-1]
		stub.name, stub.args = name, args
		return s
	}
}

// CombinedOutput runs the command and returns its combined standard
// output and standard error.
func (s *FakeExecutables) CombinedOutput() ([]byte, error) {
	stub := s.Stubs[s.index-1]
	s.recordedCommands = append(s.recordedCommands, stub.Command())
	return stub.Bytes, stub.Err
}

// Command returns the command arguments
func (s *FakeExecutables) Command() []string {
	stub := s.Stubs[s.index-1]
	return append([]string{stub.name}, stub.args...)
}

// StdOut returns the stdout
func (s *FakeExecutables) StdOut() []byte {
	stub := s.Stubs[s.index-1]
	return stub.StdOutput
}

// StdErr returns the stderr
func (s *FakeExecutables) StdErr() []byte {
	stub := s.Stubs[s.index-1]
	return stub.StdError
}

// Run returns an err
func (s *FakeExecutables) Run() error {
	stub := s.Stubs[s.index-1]
	s.recordedCommands = append(s.recordedCommands, stub.Command())
	return stub.Err
}

// RecordedExecution returns the a slice of string slices with the recorded
// commands executed.
func (s *FakeExecutables) RecordedExecution() [][]string { return s.recordedCommands }

// FakeExecutable implements the Executable interface providing all the
// required properties to be able to stub any cmd command delegation.
type FakeExecutable struct {
	name                string
	args                []string
	Bytes               []byte
	StdError, StdOutput []byte
	Err                 error
}

// NewFakeCmdBuilder initializes a FakeExecutable and returns a ExecutableFactory
// signature.
func NewFakeCmdBuilder(s *FakeExecutable) ExecutableFactory {
	return func(name string, args ...string) Executable {
		s.name, s.args = name, args
		return s
	}
}

// CombinedOutput runs the command and returns its combined standard
// output and standard error.
func (s *FakeExecutable) CombinedOutput() ([]byte, error) {
	return s.Bytes, s.Err
}

// Command returns the command arguments
func (s *FakeExecutable) Command() []string {
	return append([]string{s.name}, s.args...)
}

// StdOut returns the stdout
func (s *FakeExecutable) StdOut() []byte { return s.StdOutput }

// StdErr returns the stderr
func (s *FakeExecutable) StdErr() []byte { return s.StdError }

// Run returns an err
func (s *FakeExecutable) Run() error { return s.Err }
