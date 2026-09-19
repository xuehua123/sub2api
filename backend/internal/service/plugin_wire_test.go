//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProvidePluginManagerBindsAccountDirectory(t *testing.T) {
	gateway := &OpenAIGatewayService{}
	manager := ProvidePluginManager(nil, nil, &config.Config{}, PluginHostInfo{}, nil, gateway)
	require.Same(t, gateway, manager.accountDirectory)
}
