package k8s

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultTimeout is a reasonable timeout to use with the Client.
const DefaultTimeout = 1 * time.Minute

// WrapClient returns a Client that performs requests within DefaultTimeout.
func WrapClient(client client.Client) Client {
	return &clientWrapper{
		crClient: client,
		timeout:  DefaultTimeout,
	}
}

// Client wraps a controller-runtime client to use a
// default context with a timeout if no context is passed.
type Client interface {
	// WithContext returns a client configured to use the provided context on
	// subsequent requests, instead of one created from the preconfigured timeout.
	WithContext(ctx context.Context) Client
	// WithTimeout returns a client with an overriden timeout value,
	// to be used when no explicit context is passed.
	WithTimeout(timeout time.Duration) Client

	// Get wraps a controller-runtime client.Get call with a context.
	Get(key client.ObjectKey, obj runtime.Object) error
	// List wraps a controller-runtime client.List call with a context.
	List(opts *client.ListOptions, list runtime.Object) error
	// Create wraps a controller-runtime client.Create call with a context.
	Create(obj runtime.Object) error
	// Delete wraps a controller-runtime client.Delete call with a context.
	Delete(obj runtime.Object, opts ...client.DeleteOptionFunc) error
	// Update wraps a controller-runtime client.Update call with a context.
	Update(obj runtime.Object) error
	// Status wraps a controller-runtime client.Status call.
	Status() StatusWriter
}

type clientWrapper struct {
	crClient client.Client
	timeout  time.Duration
	ctx      context.Context // nil if not provided
}

// WithContext returns a client configured to use the provided context on
// subsequent requests, instead of one created from the preconfigured timeout.
func (w *clientWrapper) WithContext(ctx context.Context) Client {
	return &clientWrapper{
		crClient: w.crClient,
		ctx:      ctx,
	}
}

// WithTimeout returns a client with an overriden timeout value,
// to be used when no explicit context is passed.
func (w *clientWrapper) WithTimeout(timeout time.Duration) Client {
	return &clientWrapper{
		crClient: w.crClient,
		timeout:  timeout,
	}
}

// callWithContext calls f with the user-provided context. If no context was
// provided, it uses the default one.
func (w *clientWrapper) callWithContext(f func(ctx context.Context) error) error {
	var ctx context.Context
	if w.ctx != nil {
		// use the provided context
		ctx = w.ctx
	} else {
		// no context provided, use the default one
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), w.timeout)
		defer cancel()
	}
	return f(ctx)
}

// Get wraps a controller-runtime client.Get call with a context.
func (w *clientWrapper) Get(key client.ObjectKey, obj runtime.Object) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.Get(ctx, key, obj)
	})
}

// List wraps a controller-runtime client.List call with a context.
func (w *clientWrapper) List(opts *client.ListOptions, list runtime.Object) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.List(ctx, opts, list)
	})
}

// Create wraps a controller-runtime client.Create call with a context.
func (w *clientWrapper) Create(obj runtime.Object) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.Create(ctx, obj)
	})
}

// Update wraps a controller-runtime client.Update call with a context.
func (w *clientWrapper) Update(obj runtime.Object) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.Update(ctx, obj)
	})
}

// Delete wraps a controller-runtime client.Delete call with a context.
func (w *clientWrapper) Delete(obj runtime.Object, opts ...client.DeleteOptionFunc) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.Delete(ctx, obj, opts...)
	})
}

// StatusWriter wraps a client.StatusWrapper with a context.
type StatusWriter struct {
	client.StatusWriter
	w *clientWrapper
}

// Status wraps a controller-runtime client.Status call.
func (w *clientWrapper) Status() StatusWriter {
	return StatusWriter{
		StatusWriter: w.crClient.Status(),
		w:            w,
	}
}

// Update wraps a controller-runtime client.Status().Update call with a context.
func (s StatusWriter) Update(obj runtime.Object) error {
	return s.w.callWithContext(func(ctx context.Context) error {
		return s.StatusWriter.Update(ctx, obj)
	})
}
