module github.com/sunshine69/automation-go

go 1.25

replace github.com/nikolalohinski/gonja/v2 => github.com/sunshine69/gonja/v2 v2.3.2

replace github.com/sunshine69/automation-go/lib => ./lib

require (
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/google/uuid v1.6.0
	github.com/hirochachacha/go-smb2 v1.1.0
	github.com/json-iterator/go v1.1.12
	github.com/klauspost/compress v1.18.3
	github.com/mattn/go-isatty v0.0.20
	github.com/nikolalohinski/gonja/v2 v2.5.1
	github.com/pkg/errors v0.9.1
	github.com/relex/aini v1.6.0
	github.com/spf13/viper v1.21.0
	github.com/sunshine69/golang-tools/utils v0.0.0-20260124100451-9e131c6e7405
	gopkg.in/ini.v1 v1.67.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/geoffgarside/ber v1.2.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/samber/lo v1.38.1 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96 // indirect
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spf13/pflag v1.0.10
	github.com/tidwall/gjson v1.18.0
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
)
