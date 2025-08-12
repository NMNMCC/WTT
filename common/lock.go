package common

import "sync"

type RWLock[V any] struct {
	l sync.RWMutex
	v V
}

func NewRWLock[V any](v V) *RWLock[V] {
	return &RWLock[V]{l: sync.RWMutex{}, v: v}
}

func (l *RWLock[V]) Read(f func(V)) {
	l.l.RLock()
	defer l.l.RUnlock()
	f(l.v)
}

func (l *RWLock[V]) Write(f func(V)) {
	l.l.Lock()
	defer l.l.Unlock()
	f(l.v)
}
