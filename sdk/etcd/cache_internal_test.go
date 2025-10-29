package etcd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCache_backoffDuration(t *testing.T) {
	c := &Cache{backoffBase: 250 * time.Millisecond, backoffMax: 2 * time.Second}
	assert.Equal(t, 250*time.Millisecond, c.backoffDuration(1))
	assert.Equal(t, 500*time.Millisecond, c.backoffDuration(2))
	assert.Equal(t, 1*time.Second, c.backoffDuration(3))
	assert.Equal(t, 2*time.Second, c.backoffDuration(4))
	assert.Equal(t, 2*time.Second, c.backoffDuration(10))
}
