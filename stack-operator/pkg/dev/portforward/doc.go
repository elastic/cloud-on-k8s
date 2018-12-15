// package portforward provides a dialer that uses the Kubernetes API to automatically forward connections to
// Kubernetes services and pods using the "port-forward" feature of kubectl.
//
// This is convenient when running outside of Kubernetes while still requiring having TCP-level access to certain
// services and pods running inside of Kubernetes.
//
// Note: It is intended for development use only.
package portforward
