package cmdutil

// StubExecutables Wraps around the StubExecutable to provide multiple stubs in
// a test.
type StubExecutables struct {
	i, index int
	stubs    []*StubExecutable
}

// NewStubsFactoryFunc initializes a StubExecutables and returns a FactoryFunc
// signature.
func NewStubsFactoryFunc(stubs []*StubExecutable) FactoryFunc {
	var s = &StubExecutables{stubs: stubs}
	return s.generateFunc()
}

func (s *StubExecutables) generateFunc() FactoryFunc {
	return func(name string, args ...string) Executable {
		if s.i > 0 {
			s.index++
		}
		s.i++
		stub := s.stubs[s.index]
		stub.name, stub.args = name, args
		return s
	}
}

// CombinedOutput runs the command and returns its combined standard
// output and standard error.
func (s *StubExecutables) CombinedOutput() ([]byte, error) {
	stub := s.stubs[s.index]
	return stub.Bytes, stub.Err
}

// Command returns the command arguments
func (s *StubExecutables) Command() []string {
	stub := s.stubs[s.index]
	return append([]string{stub.name}, stub.args...)
}

// StdOut returns the stdout
func (s *StubExecutables) StdOut() []byte {
	stub := s.stubs[s.index]
	return stub.StdOutput
}

// StdErr returns the stderr
func (s *StubExecutables) StdErr() []byte {
	stub := s.stubs[s.index]
	return stub.StdError
}

// Run returns an err
func (s *StubExecutables) Run() error {
	stub := s.stubs[s.index]
	return stub.Err
}

// StubExecutable implements the Executable interface providing all the
// required properties to be able to stub any cmd command delegation.
type StubExecutable struct {
	name                string
	args                []string
	Bytes               []byte
	StdError, StdOutput []byte
	Err                 error
}

// NewStubFactoryFunc initializes a StubExecutable and returns a FactoryFunc
// signature.
func NewStubFactoryFunc(s *StubExecutable) FactoryFunc {
	return func(name string, args ...string) Executable {
		s.name, s.args = name, args
		return s
	}
}

// CombinedOutput runs the command and returns its combined standard
// output and standard error.
func (s *StubExecutable) CombinedOutput() ([]byte, error) {
	return s.Bytes, s.Err
}

// Command returns the command arguments
func (s *StubExecutable) Command() []string {
	return append([]string{s.name}, s.args...)
}

// StdOut returns the stdout
func (s *StubExecutable) StdOut() []byte { return s.StdOutput }

// StdErr returns the stderr
func (s *StubExecutable) StdErr() []byte { return s.StdError }

// Run returns an err
func (s *StubExecutable) Run() error { return s.Err }
