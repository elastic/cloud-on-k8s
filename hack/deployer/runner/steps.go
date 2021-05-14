package runner


func run(steps []func() error) error {
	for _, fn := range steps {
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}
