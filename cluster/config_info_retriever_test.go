package cluster

import (
	"testing"

	"github.com/echo766/pitaya/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestConfigInfoRetrieverRegion(t *testing.T) {
	t.Parallel()

	c := viper.New()
	c.Set("pitaya.cluster.info.region", "us")
	config := config.NewConfig(c)

	infoRetriever := NewConfigInfoRetriever(config)

	assert.Equal(t, "us", infoRetriever.Region())
}
