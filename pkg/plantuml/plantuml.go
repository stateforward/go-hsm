package plantuml

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/stateforward/go-hsm/elements"
	"github.com/stateforward/go-hsm/kinds"
	"github.com/stateforward/go-hsm/pkg/set"
)

func idFromQualifiedName(qualifiedName string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(qualifiedName, "/"), ".", "_"), "/", ".")
}

func labelFromName(name string) string {
	return strings.TrimPrefix(name, ".")
}

func generateState(builder *strings.Builder, depth int, state elements.Element, allElements []elements.Element, visited set.Set[string]) {
	id := idFromQualifiedName(state.QualifiedName())
	indent := strings.Repeat(" ", depth*2)
	composite := false
	visited.Add(state.QualifiedName())
	for _, element := range allElements {
		if visited.Contains(element.QualifiedName()) {
			continue
		}
		if element.Owner() == state.QualifiedName() {
			if kinds.IsKind(element.Kind(), kinds.Vertex) {
				if !composite {
					composite = true
					fmt.Fprintf(builder, "%sstate %s{\n", indent, id)
				}
				slog.Info("generateVertex", "for", state.QualifiedName(), "element", element.QualifiedName())
				generateVertex(builder, depth+1, element, allElements, visited)
			} else if kinds.IsKind(element.Kind(), kinds.Transition) {

				transition := element.(elements.Transition)
				if strings.HasSuffix(transition.Source(), ".initial") {
					if !composite {
						composite = true
						fmt.Fprintf(builder, "%sstate %s{\n", indent, id)
					}
					generateTransition(builder, depth+1, transition, allElements, visited)
				}
			}
		}
	}
	if composite {
		fmt.Fprintf(builder, "%s}\n", indent)
	} else {
		tag := ""
		if kinds.IsKind(state.Kind(), kinds.Choice) {
			tag = " <<choice>> "
		}
		fmt.Fprintf(builder, "%sstate %s%s\n", indent, id, tag)
	}
}

func generateVertex(builder *strings.Builder, depth int, vertex elements.Element, allElements []elements.Element, visited set.Set[string]) {
	if kinds.IsKind(vertex.Kind(), kinds.State) {
		generateState(builder, depth, vertex, allElements, visited)
	}
}

func generateTransition(builder *strings.Builder, depth int, transition elements.Transition, _ []elements.Element, visited set.Set[string]) {
	visited.Add(transition.QualifiedName())
	if kinds.IsKind(transition.Kind(), kinds.Internal) {
		return
	}
	source := transition.Source()
	events := ""
	if strings.HasSuffix(source, ".initial") {
		source = "[*]"
	} else {
		if len(transition.Events()) > 0 {
			names := []string{}
			for _, event := range transition.Events() {
				names = append(names, event.Name())
			}
			events = fmt.Sprintf(": %s", strings.Join(names, "|"))
		}
	}
	target := transition.Target()
	indent := strings.Repeat(" ", depth*2)
	fmt.Fprintf(builder, "%s%s ----> %s%s\n", indent, idFromQualifiedName(source), idFromQualifiedName(target), events)
}

func generateElements(builder *strings.Builder, depth int, allElements []elements.Element, visited set.Set[string]) {
	fmt.Fprintln(builder, "@startuml")
	for _, element := range allElements {
		if visited.Contains(element.QualifiedName()) {
			continue
		}
		if kinds.IsKind(element.Kind(), kinds.State, kinds.Choice) {
			generateState(builder, depth+1, element, allElements, visited)
		}
	}
	for _, element := range allElements {
		if kinds.IsKind(element.Kind(), kinds.Transition) {
			transition := element.(elements.Transition)
			if !strings.HasSuffix(transition.Source(), ".initial") {
				generateTransition(builder, depth, transition, allElements, visited)
			}
		}
	}
	fmt.Fprintln(builder, "@enduml")
}

func Generate(model elements.Model) string {

	var builder strings.Builder
	elements := []elements.Element{}
	for _, element := range model.Elements() {
		elements = append(elements, element)
	}
	// Sort elements hierarchically like a directory structure
	sort.Slice(elements, func(i, j int) bool {
		iPath := strings.Split(elements[i].QualifiedName(), "/")
		jPath := strings.Split(elements[j].QualifiedName(), "/")

		// Compare each path segment
		minLen := len(iPath)
		if len(jPath) < minLen {
			minLen = len(jPath)
		}

		for k := 0; k < minLen; k++ {
			if iPath[k] != jPath[k] {
				return iPath[k] < jPath[k]
			}
		}

		// If all segments match up to minLen, shorter path comes first
		return len(iPath) < len(jPath)
	})

	generateElements(&builder, 0, elements, set.New[string]())
	slog.Info("Generate", "builder", builder.String())
	return builder.String()
}
