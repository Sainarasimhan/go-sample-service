// Package config - Configuration Module
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/mitchellh/mapstructure"
	viper "github.com/spf13/viper"
)

//Cfg as Structure ?? -- trial
type Cfg struct {
	Sample SampleCfg `yaml:"Sample"`
	DB     DBCfg     `config:"DB"`
}

//Pwd's and certs (all secrets) should be obtained from secret manager..

//AuditCfg - Config for Audit service
type SampleCfg struct {
	// Update API Configurations
	GRPC APICfg `yaml:"grpc-API"`
	// Get API Configurations
	HTTP APICfg `yaml:"http-API"`
	// Prometheus config
	Prometheus APICfg `yaml:"Prom-API"`
	//Zipkin Config
	Zipkin APICfg `yaml:"Zipkin-API"`
	//PubSub config
	PS PubSubCfg `yaml:"PubSub"`
	//Concurrent Writes
	ConcurrentWrites int `yaml:"Concurrent-Writes"`
	// Error reporting
	ErrRprtPrjID string `yaml:"ErrorReportProject" valid:"optional"`
	// Log config
	Log LogCfg `yaml:"Log"`
}

//APICfg - Config for each API server
type APICfg struct {
	// Server port and cert details

	// Server Name
	Name string `yaml:"Name"`
	// Port to Listen
	Port int `yaml:"Port" valid:"numeric,required"`
	//URL
	URL string `yaml:"URL"`
	// Secure or not
	Secure bool `yaml:"Secure"`
	// certs
	TLSCert string `yaml:"Cert" valid:"optional"`
	// cert Key
	TLSKey string `yaml:"Key" valid:"optional"`
}

//PubSubCfg - Config related to GCP pub/sub
type PubSubCfg struct {
	SubURL  string `yaml:"SubURL" valid:"optional"`
	PubURL  string `yaml:"PubURL" valid:"optional"`
	Enabled bool   `yaml:"Enable" valid:"required"`
	// Time out and ack config to be added
}

// LogCfg - Config for Logging
type LogCfg struct {
	Level  string `yaml:"Level" valid:"required"` // Add Validation on levels
	Format string `yaml:"Format" valid:"alphanum, required"`
	Name   string `yaml:"Name" valid:"alphanum"`
}

//DBCfg - Config related to DB connectivity
type DBCfg struct {
	//Db Conn
	Driver   string `config:"Driver" valid:"required"`
	HName    string `config:"HostName" valid:"required"`
	Database string `config:"Database" valid:"alphanum,required"`
	Port     int    `config:"Port"     valid:"numeric,required"`
	UID      string `config:"UserName" valid:"alphanum, required"`
	Pwd      string `config:"Password" valid:"alphanum, required"`
	GcpSM    string `config:"PasswordGCP" valid:"required"`
	MaxConns int    `config:"MaxConnections" valid:"numeric, required"`
	Log      LogCfg `config:"Log"`
}

//Init - Func to init cfg by reading config files
func Init() (cfg *Cfg, err error) {

	cfg = &Cfg{}
	if err = getSvcCfg(cfg); err != nil {
		return
	}
	err = getDBCfg(cfg)
	return
}

func getSvcCfg(cfg *Cfg) (err error) {

	// Read From config file
	resPath := "resources/" + os.Getenv("APPENV") + "/"
	viper.SetConfigName("application")
	viper.AddConfigPath("./" + resPath)
	viper.AddConfigPath("../" + resPath)
	viper.AddConfigPath("../../" + resPath)

	err = viper.ReadInConfig()
	if err != nil {
		return // return err if config file is not found
	}

	// Set Default and env based variables ..
	/*defFlags := servicelog.LstdFlags | servicelog.Lmicroseconds
	viper.SetDefault("audit.log.flags", defFlags)
	viper.SetDefault("audit.grpc-api.log.flags", defFlags)
	viper.SetDefault("audit.http-api.log.flags", defFlags) */

	// yaml parsing to cfg struct
	err = viper.Unmarshal(&cfg, func(m *mapstructure.DecoderConfig) {
		m.TagName = "yaml"
	})
	if err != nil {
		return //return err if unmarshal fails
	}

	// Set PORT for http/grpc
	transport := os.Getenv("APPTRANSPORT")
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	if transport == "HTTP" {
		cfg.Sample.HTTP.Port = port
	} else if transport == "GRPC" {
		cfg.Sample.GRPC.Port = port
	}

	// yaml validation for log level
	//govalidator.TagMap["Level"] = govalidator.Validator(valLevel)

	var ok bool
	if ok, err = govalidator.ValidateStruct(&cfg.Sample); !ok {
		err = fmt.Errorf("Service Cfg Validation Error \n\t -- %w", err)
	}

	return
}

func getDBCfg(cfg *Cfg) (err error) {
	// Create DB specific viper
	dbCfg := viper.New()
	// Read from DB2 file
	dbPath := "resources/" + os.Getenv("APPENV") + "/db/"
	dbCfg.SetConfigName("db")
	dbCfg.AddConfigPath("./" + dbPath)
	dbCfg.AddConfigPath("../" + dbPath)
	dbCfg.AddConfigPath("./db/")

	// Set Default and env based variables ..
	dbCfg.SetEnvPrefix("Sample")
	dbCfg.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	dbCfg.BindEnv("db.username")
	dbCfg.BindEnv("db.password")

	err = dbCfg.ReadInConfig()
	if err != nil {
		return // return err if config file is not found
	}

	// yaml parsing to cfg struct
	err = dbCfg.Unmarshal(&cfg,
		func(m *mapstructure.DecoderConfig) {
			m.TagName = "config"
		})
	if err != nil {
		return //return err if unmarshal fails
	}

	// Get Pwd from GCP KMS
	cfg.DB.Pwd, err = gcpDBPwd(cfg.DB.GcpSM) // Pass Secret Name
	if err != nil {
		return
	}

	// yaml validation
	var ok bool
	if ok, err = govalidator.ValidateStruct(&cfg.DB); !ok {
		err = fmt.Errorf("DB Cfg Validation Error \n\t -- %w", err)
	}

	return
}

/*func valLevel(level string) bool {
	return level == servicelog.LevelError ||
		level == servicelog.LevelInfo ||
		level == servicelog.LevelDebug
}*/

//Address -- return Address as string
func (a APICfg) Address() string {
	if a.URL != "" {
		return a.URL
	}
	return fmt.Sprintf("%s:%d", a.Name, a.Port)
}

//ConnString -- returns DB connection string
func (d DBCfg) ConnString() string {
	return fmt.Sprintf("HOSTNAME=%s;DATABASE=%s;PORT=%d;UID=%s;PWD=%s", d.HName, d.Database, d.Port, d.UID, d.Pwd)
}

// PostgresStr - Returns DB connection string for Postgres
func (d DBCfg) PostgresStr() string {
	return fmt.Sprintf("user=%s password=%s host=%s dbname=%s sslmode=%s", d.UID, d.Pwd, d.HName, d.Database, "disable")
}
