// TODO: This code is hacked together and needs to be refactored.
// It generates PlantUML diagrams but the structure and organization could be improved.
package plantuml

import (
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/stateforward/go-hsm/elements"
	"github.com/stateforward/go-hsm/kinds"
)

func idFromQualifiedName(qualifiedName string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(qualifiedName, "/"), "."), ".", "_"), "/", ".")
}

func generateState(builder *strings.Builder, depth int, state elements.Element, model elements.Model, allElements []elements.Element, visited map[string]any) {
	id := idFromQualifiedName(state.QualifiedName())
	indent := strings.Repeat(" ", depth*2)
	composite := false
	visited[state.QualifiedName()] = struct{}{}
	for _, element := range allElements {
		if _, ok := visited[element.QualifiedName()]; ok {
			continue
		}
		if element.Owner() == state.QualifiedName() {
			if kinds.IsKind(element.Kind(), kinds.Vertex) {
				if !composite {
					composite = true
					fmt.Fprintf(builder, "%sstate %s{\n", indent, id)
				}
				generateVertex(builder, depth+1, element, model, allElements, visited)
			}
		}
	}
	initial, ok := model.Namespace()[path.Join(state.QualifiedName(), ".initial")]
	if ok {
		if !composite {
			composite = true
			fmt.Fprintf(builder, "%sstate %s{\n", indent, id)
		}
		if transition, ok := model.Namespace()[initial.(elements.Vertex).Transitions()[0]]; ok {
			generateTransition(builder, depth+1, transition.(elements.Transition), allElements, visited)
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
	if kinds.IsKind(state.Kind(), kinds.State) {
		state := state.(elements.State)
		if entry := state.Entry(); entry != "" {
			fmt.Fprintf(builder, "%sstate %s: entry / %s\n", indent, id, idFromQualifiedName(path.Base(entry)))
		}
		if activity := state.Activity(); activity != "" {
			fmt.Fprintf(builder, "%sstate %s: activity / %s\n", indent, id, idFromQualifiedName(path.Base(activity)))
		}
		if exit := state.Exit(); exit != "" {
			fmt.Fprintf(builder, "%sstate %s: exit / %s\n", indent, id, idFromQualifiedName(path.Base(exit)))
		}
	}
}

func generateVertex(builder *strings.Builder, depth int, vertex elements.Element, model elements.Model, allElements []elements.Element, visited map[string]any) {
	if kinds.IsKind(vertex.Kind(), kinds.State) {
		generateState(builder, depth, vertex, model, allElements, visited)
	}
}

func generateTransition(builder *strings.Builder, depth int, transition elements.Transition, _ []elements.Element, visited map[string]any) {
	visited[transition.QualifiedName()] = struct{}{}
	if kinds.IsKind(transition.Kind(), kinds.Internal) {
		return
	}
	source := transition.Source()
	label := ""
	if strings.HasSuffix(source, ".initial") {
		source = "[*]"
	} else {
		if len(transition.Events()) > 0 {
			names := []string{}
			for _, event := range transition.Events() {
				names = append(names, idFromQualifiedName(path.Base(event.Name())))
			}
			label = strings.Join(names, "|")
		}
	}
	if guard := transition.Guard(); guard != "" {
		label = fmt.Sprintf("%s [%s]", label, idFromQualifiedName(path.Base(guard)))
	}
	if effect := transition.Effect(); effect != "" {
		label = fmt.Sprintf("%s / %s", label, idFromQualifiedName(path.Base(effect)))
	}
	if label != "" {
		label = fmt.Sprintf(" : %s", label)
	}
	target := transition.Target()
	indent := strings.Repeat(" ", depth*2)
	fmt.Fprintf(builder, "%s%s ----> %s%s\n", indent, idFromQualifiedName(source), idFromQualifiedName(target), label)
}

func generateElements(builder *strings.Builder, depth int, model elements.Model, allElements []elements.Element, visited map[string]any) {
	fmt.Fprintln(builder, "@startuml")
	for _, element := range allElements {
		if _, ok := visited[element.QualifiedName()]; ok {
			continue
		}
		if kinds.IsKind(element.Kind(), kinds.State, kinds.Choice) {
			generateState(builder, depth+1, element, model, allElements, visited)
		}
	}
	if initial, ok := model.Namespace()[path.Join(model.QualifiedName(), ".initial")]; ok {
		if transition, ok := model.Namespace()[initial.(elements.Vertex).Transitions()[0]]; ok {
			generateTransition(builder, depth, transition.(elements.Transition), allElements, visited)
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

func Generate(writer io.Writer, model elements.Model) error {
	var builder strings.Builder
	elements := []elements.Element{}
	for _, element := range model.Namespace() {
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

	generateElements(&builder, 0, model, elements, map[string]any{})
	_, err := writer.Write([]byte(builder.String()))
	return err
}
