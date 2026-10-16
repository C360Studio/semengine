package graphingest

import (
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/graphmutation"
)

func TestDefaultConfigDeclaresRequiredMutationProvider(t *testing.T) {
	config := DefaultConfig()
	for _, definition := range config.Ports.Inputs {
		port, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			t.Fatalf("resolve mutation provider: %v", err)
		}
		request, ok := port.Config.(component.NATSRequestPort)
		if !ok || request.Interface == nil || request.Interface.Type != graphmutation.InterfaceType {
			continue
		}
		if !port.Required || request.Subject != graphmutation.SubjectFamily ||
			request.Interface.Version != graphmutation.InterfaceVersion {
			t.Fatalf("mutation provider = %#v", port)
		}
		return
	}
	t.Fatal("default config has no typed graph mutation provider")
}
