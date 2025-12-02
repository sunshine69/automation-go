module github.com/sunshine69/automation-go

go 1.24.0

toolchain go1.24.3

replace github.com/nikolalohinski/gonja/v2 => github.com/sunshine69/gonja/v2 v2.3.2

replace github.com/sunshine69/automation-go/lib => ./lib

require (
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/hirochachacha/go-smb2 v1.1.0
	github.com/json-iterator/go v1.1.12
	github.com/klauspost/compress v1.18.2
	github.com/mattn/go-isatty v0.0.20
	github.com/nikolalohinski/gonja/v2 v2.4.2
	github.com/pkg/errors v0.9.1
	github.com/spf13/viper v1.21.0
	github.com/sunshine69/golang-tools/utils v0.0.0-20251202041847-e729b60796a0
	gopkg.in/ini.v1 v1.67.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/geoffgarside/ber v1.2.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20251125195548-87e1e737ad39 // indirect
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/pflag v1.0.10
	github.com/tidwall/gjson v1.18.0
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
)
