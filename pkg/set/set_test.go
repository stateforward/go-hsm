package set_test

import (
	"testing"

	"github.com/stateforward/go-hsm/pkg/set"
)

func TestSet(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		s := set.New[string]("a", "b", "c")
		if s == nil {
			t.Error("Expected non-nil set")
		}
		if s.Size() != 3 {
			t.Errorf("Expected size 3, got %d", s.Size())
		}
		if !s.Contains("a") {
			t.Error("Expected set to contain 'a'")
		}
		if !s.Contains("b") {
			t.Error("Expected set to contain 'b'")
		}
		if !s.Contains("c") {
			t.Error("Expected set to contain 'c'")
		}
	})

	t.Run("Add", func(t *testing.T) {
		s := set.Set[string]{}
		s.Add("test")
		if s.Size() != 1 {
			t.Errorf("Expected size 1, got %d", s.Size())
		}
		if !s.Contains("test") {
			t.Error("Expected set to contain 'test'")
		}
	})

	t.Run("Remove", func(t *testing.T) {
		s := set.Set[string]{}
		s.Add("test")
		s.Remove("test")
		if s.Size() != 0 {
			t.Errorf("Expected size 0, got %d", s.Size())
		}
		if s.Contains("test") {
			t.Error("Expected set to not contain 'test'")
		}
	})

	t.Run("Contains", func(t *testing.T) {
		s := set.Set[string]{}
		if s.Contains("test") {
			t.Error("Expected set to not contain 'test'")
		}
		s.Add("test")
		if !s.Contains("test") {
			t.Error("Expected set to contain 'test'")
		}
	})

	t.Run("Size", func(t *testing.T) {
		s := set.Set[string]{}
		if s.Size() != 0 {
			t.Errorf("Expected size 0, got %d", s.Size())
		}
		s.Add("test1")
		if s.Size() != 1 {
			t.Errorf("Expected size 1, got %d", s.Size())
		}
		s.Add("test2")
		if s.Size() != 2 {
			t.Errorf("Expected size 2, got %d", s.Size())
		}
	})

	t.Run("Clear", func(t *testing.T) {
		s := set.Set[string]{}
		s.Add("test1")
		s.Add("test2")
		s.Clear()
		if s.Size() != 0 {
			t.Errorf("Expected size 0, got %d", s.Size())
		}
	})

	t.Run("Items", func(t *testing.T) {
		s := set.Set[string]{}
		s.Add("test1")
		s.Add("test2")
		s.Add("test3")

		items := make(map[string]bool)
		for item := range s.Items() {
			if item == "test3" {
				break
			}
			items[item] = true
		}
		if len(items) != 2 {
			t.Errorf("Expected 2 items, got %d", len(items))
		}
		if !items["test1"] {
			t.Error("Expected items to contain 'test1'")
		}
		if !items["test2"] {
			t.Error("Expected items to contain 'test2'")
		}
		if items["test3"] {
			t.Error("Expected items to not contain 'test3'")
		}
	})

	t.Run("Union", func(t *testing.T) {
		s1 := set.Set[string]{}
		s1.Add("test1")
		s1.Add("test2")

		s2 := set.Set[string]{}
		s2.Add("test2")
		s2.Add("test3")

		union := s1.Union(s2)
		if union.Size() != 3 {
			t.Errorf("Expected size 3, got %d", union.Size())
		}
		if !union.Contains("test1") {
			t.Error("Expected union to contain 'test1'")
		}
		if !union.Contains("test2") {
			t.Error("Expected union to contain 'test2'")
		}
		if !union.Contains("test3") {
			t.Error("Expected union to contain 'test3'")
		}
	})

	t.Run("Intersection", func(t *testing.T) {
		s1 := set.Set[string]{}
		s1.Add("test1")
		s1.Add("test2")

		s2 := set.Set[string]{}
		s2.Add("test2")
		s2.Add("test3")

		intersection := s1.Intersection(s2)
		if intersection.Size() != 1 {
			t.Errorf("Expected size 1, got %d", intersection.Size())
		}
		if !intersection.Contains("test2") {
			t.Error("Expected intersection to contain 'test2'")
		}
	})

	t.Run("Difference", func(t *testing.T) {
		s1 := set.Set[string]{}
		s1.Add("test1")
		s1.Add("test2")

		s2 := set.Set[string]{}
		s2.Add("test2")
		s2.Add("test3")

		difference := s1.Difference(s2)
		if difference.Size() != 1 {
			t.Errorf("Expected size 1, got %d", difference.Size())
		}
		if !difference.Contains("test1") {
			t.Error("Expected difference to contain 'test1'")
		}
	})
}
