package user

import (
	"sync"

	"golang.org/x/crypto/bcrypt"

	"k8s.io/apimachinery/pkg/types"
)

type PasswordCheck interface {

	// CompareHashAndPassword compares a bcrypt hashed password with its possible
	// plaintext equivalent. Returns nil on success, or an error on failure.
	CompareHashAndPassword(association types.NamespacedName, hashedPassword, password []byte) error

	// Remove the cached password for the association
	Remove(association types.NamespacedName)
}

type hashedAndClearPassword struct {
	hashedPassword, password []byte
}

type CachedPasswordCheck struct {
	mutex sync.RWMutex
	cache map[types.NamespacedName]*hashedAndClearPassword
}

func (c *CachedPasswordCheck) CompareHashAndPassword(association types.NamespacedName, hashedPassword, password []byte) error {
	p := c.getHashedAndClearPassword(association)
	if p == nil {

	}
	return nil
}

func (c *CachedPasswordCheck) Remove(association types.NamespacedName) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.cache, association)
}

func (c *CachedPasswordCheck) updateHashedAndClearPassword(association types.NamespacedName, hashedPassword, password []byte) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if bcrypt.CompareHashAndPassword(hashedPassword, password) == nil {
		// Hash and password match, keep it in memory
		hcp := hashedAndClearPassword{
			hashedPassword: hashedPassword,
			password:       password,
		}
		c.cache[association] = &hcp
	}
}

func (c *CachedPasswordCheck) getHashedAndClearPassword(association types.NamespacedName) *hashedAndClearPassword {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	// Try to read the passwors
	if p, ok := c.cache[association]; ok {
		return p
	}
	return nil
}
