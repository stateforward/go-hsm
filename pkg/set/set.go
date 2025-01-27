package set

import (
	"iter"
)

type Set[T comparable] map[T]struct{}

func New[T comparable](items ...T) Set[T] {
	s := make(Set[T])
	for _, item := range items {
		s[item] = struct{}{}
	}
	return s
}

// Add adds an item to the set
func (s Set[T]) Add(items ...T) {
	for _, item := range items {
		s[item] = struct{}{}
	}
}

// Remove removes an item from the set
func (s Set[T]) Remove(item T) {
	delete(s, item)
}

// Contains checks if an item exists in the set
func (s Set[T]) Contains(item T) bool {
	_, exists := s[item]
	return exists
}

func (s Set[T]) ContainsAll(items ...T) bool {
	for _, item := range items {
		if !s.Contains(item) {
			return false
		}
	}
	return true
}

func (s Set[T]) ContainsAny(items ...T) bool {
	for _, item := range items {
		if s.Contains(item) {
			return true
		}
	}
	return false
}

// Size returns the number of items in the set
func (s Set[T]) Size() int {
	return len(s)
}

// Clear removes all items from the set
func (s Set[T]) Clear() {
	for k := range s {
		delete(s, k)
	}
}

// Items returns all items in the set as a sequence
func (s Set[T]) Items() iter.Seq[T] {
	return func(yield func(T) bool) {
		for item := range s {
			if !yield(item) {
				return
			}
		}
	}
}

// Union returns a new set containing all items from both sets
func (s Set[T]) Union(other Set[T]) Set[T] {
	result := make(Set[T])
	for item := range s {
		result[item] = struct{}{}
	}
	for item := range other {
		result[item] = struct{}{}
	}
	return result
}

// Intersection returns a new set containing items present in both sets
func (s Set[T]) Intersection(other Set[T]) Set[T] {
	result := make(Set[T])
	for item := range s {
		if other.Contains(item) {
			result[item] = struct{}{}
		}
	}
	return result
}

// Difference returns a new set containing items in s that are not in other
func (s Set[T]) Difference(other Set[T]) Set[T] {
	result := make(Set[T], len(s))
	for item := range s {
		if !other.Contains(item) {
			result[item] = struct{}{}
		}
	}
	return result
}
