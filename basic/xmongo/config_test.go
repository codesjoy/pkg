// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package xmongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	var nilCfg *Config
	require.ErrorIs(t, nilCfg.Validate(), ErrEmptyURI)

	cfg := Config{}
	require.ErrorIs(t, cfg.Validate(), ErrEmptyURI)

	cfg = Config{URI: "   "}
	require.ErrorIs(t, cfg.Validate(), ErrEmptyURI)

	cfg = Config{URI: " mongodb://127.0.0.1:27017 "}
	require.NoError(t, cfg.Validate())
	require.Equal(t, "mongodb://127.0.0.1:27017", cfg.URI)

	cfg = Config{
		URI:             "mongodb://127.0.0.1:27017",
		DefaultDatabase: " app ",
	}
	require.NoError(t, cfg.Validate())
	require.Equal(t, "app", cfg.DefaultDatabase)

	cfg = Config{
		URI:             "mongodb://127.0.0.1:27017",
		DefaultDatabase: "   ",
	}
	require.ErrorIs(t, cfg.Validate(), ErrEmptyDefaultDatabase)

	cfg = Config{
		URI:           "mongodb://127.0.0.1:27017",
		ClientOptions: []*options.ClientOptions{nil},
	}
	require.ErrorIs(t, cfg.Validate(), ErrNilClientOption)

	cfg = Config{
		URI: "mongodb://127.0.0.1:27017",
		ClientOptions: []*options.ClientOptions{
			options.Client().SetMinPoolSize(10).SetMaxPoolSize(1),
		},
	}
	require.Error(t, cfg.Validate())
}
