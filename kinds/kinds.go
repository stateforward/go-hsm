package kinds

const (
	length   = 64
	idLength = 8
	depthMax = length / idLength
	idMask   = (1 << idLength) - 1
)

// TypeBases returns the "base" IDs at each level
// (beyond the first) by shifting and masking.
func Bases(t uint64) [depthMax]uint64 {
	var bases [depthMax]uint64
	for i := 1; i < depthMax; i++ {
		bases[i-1] = (t >> (idLength * i)) & idMask
	}
	return bases
}

func Kind(id uint64, bases ...uint64) uint64 {
	id = id & idMask
	ids := make(map[uint64]struct{})

	for _, base := range bases {
		for j := 0; j < depthMax; j++ {
			baseId := (base >> (idLength * j)) & idMask
			if baseId == 0 {
				break
			}
			if _, ok := ids[baseId]; !ok {
				ids[baseId] = struct{}{}
				id |= baseId << (idLength * len(ids))
			}
		}
	}
	return id
}

// IsBase checks if 'typeVal' matches any or all bases provided.
func IsKind(kind uint64, bases ...uint64) bool {
	for _, base := range bases {
		baseId := base & idMask
		if kind == baseId {
			return true
		}
		for i := 0; i < depthMax; i++ {
			currentId := (kind >> (idLength * i)) & idMask
			if currentId == baseId {
				return true
			}
		}
	}
	return false
}

var (
	Null         = Kind(0)
	Element      = Kind(1)
	Vertex       = Kind(2, Element)
	Constraint   = Kind(3, Element)
	Behavior     = Kind(4, Element)
	StateMachine = Kind(5, Behavior)
	State        = Kind(6, Vertex)
	Transition   = Kind(7, Element)
	Internal     = Kind(8, Transition)
	External     = Kind(9, Transition)
	Local        = Kind(10, Transition)
	Self         = Kind(11, Transition)
	Event        = Kind(12, Element)
	TimeEvent    = Kind(13, Event)
	Concurrent   = Kind(14, Behavior)

	PseudoState = Kind(15, Vertex)
	Initial     = Kind(16, PseudoState)
	Final       = Kind(17, PseudoState)
	Choice      = Kind(18, PseudoState)
)
