package pds

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
)

type EnvConfig struct {
	Port     uint16
	Hostname string
	Version  string
	Service  struct {
		DID  string
		Name string
	}
	HomeURL                        string
	LogoURL                        string
	PrivacyPolicyURL               string
	SupportURL                     string
	TermsOfServiceURL              string
	ContactEmailAddress            string
	AcceptingRepoImports           bool
	BlobUploadLimit                int
	DevMode                        bool
	LogEnabled                     bool   `env:"LOG_ENABLED,noprefix"`
	LogLevel                       string `env:"LOG_LEVEL,noprefix"`
	DataDirectory                  string
	AccountDBLocation              string
	SequencerDBLocation            string
	DIDCacheDBLocation             string
	SqliteDisableWALAutoCheckpoint bool
	ActorStore                     struct {
		Directory string
		CacheSize int
	}
	BlobstoreS3   *EnvBlobstoreS3
	BlobstoreDisk *EnvBlobstoreDisk
	DidPlcURL     string
	DIDCache      struct {
		StaleTTL int
		MaxTTL   int
	}
	ResolverTimeout         int
	IDResolverTimeout       int
	RecoveryDIDKey          string
	ServiceHandleDomains    []string
	HandleBackupNameservers []string
	EnableDIDDocWithSession bool
	Entryway                *EnvEntrywayConfig
	Invite                  *EnvInviteConfig
	Email                   struct {
		SmtpURL     string
		FromAddress string
	}
	ModerationEmail struct {
		SmtpURL string
		Address string
	}
	MaxSubscriptionBuffer int
	RepoBackfillLimitMS   int
	BskyAppView           struct {
		ConfigService
		CdnURLPattern string
	}
	ModService         ConfigService
	ReportService      ConfigService
	RateLimitsEnabled  bool
	RateLimitBypassKey string
	RateLimitBypassIPS []string
	RedisScratch       struct {
		Address  string
		Password string
	}
	Crawlers       []string
	DropSecret     string
	JwtSecret      string
	AdminPassword  string
	PlcRotationKey struct {
		KmsKeyID          string
		K256PrivateKeyHex string
	}
	FetchMaxResponseSize  int
	DisableSSRFProtection bool
	Proxy                 EnvProxyConfig
}

type ConfigService struct {
	URL string
	DID string
}

type EnvBlobstoreDisk struct {
	Location    string
	TmpLocation string
}

type EnvBlobstoreS3 struct {
	Bucket          string
	Region          string
	Endpoint        string
	ForcePathStyle  bool
	AccessKeyID     string
	SecretAccessKey string
	UploadTimeoutMS int64
}

func (cs *ConfigService) URLHost() string {
	u, _ := url.Parse(cs.URL)
	if u == nil {
		return ""
	}
	return u.Host
}

func (c *EnvConfig) InitDefaults() {
	if c.Port == 0 {
		c.Port = 3000
	}
	if c.BlobUploadLimit == 0 {
		c.BlobUploadLimit = 5 * 1024 * 1024 // 5mb
	}
	if c.BlobstoreS3 != nil && c.BlobstoreS3.UploadTimeoutMS == 0 {
		c.BlobstoreS3.UploadTimeoutMS = 20000
	}
	if c.ActorStore.CacheSize == 0 {
		c.ActorStore.CacheSize = 100
	}
	if len(c.ServiceHandleDomains) == 0 {
		if c.Hostname == "localhost" {
			c.ServiceHandleDomains = []string{".test"}
		} else {
			c.ServiceHandleDomains = []string{fmt.Sprintf(".%s", c.Hostname)}
		}
	}
	d(&c.ActorStore.Directory, filepath.Join(c.DataDirectory, "actors"))
	d(&c.AccountDBLocation, filepath.Join(c.DataDirectory, "account.sqlite"))
	d(&c.SequencerDBLocation, filepath.Join(c.DataDirectory, "sequencer.sqlite"))
	d(&c.DIDCacheDBLocation, filepath.Join(c.DataDirectory, "did_cache.sqlite"))
	d(&c.LogLevel, "info")
}

func (c *EnvConfig) BlueskyDefaults() {
	d(&c.DidPlcURL, "https://plc.directory")
	d(&c.ReportService.URL, "https://mod.bsky.app")
	d(&c.ReportService.DID, "did:plc:ar7c4by46qjdydhdevvrndac")
	d(&c.BskyAppView.URL, "https://api.bsky.app")
	d(&c.BskyAppView.DID, "did:web:api.bsky.app")
	if len(c.Crawlers) == 0 {
		c.Crawlers = []string{"https://bsky.network"}
	}
}

func (c *EnvConfig) Validate() error {
	if len(c.DataDirectory) == 0 {
		return errors.New("PDS_DATA_DIRECTORY is required")
	}
	return nil
}

func (c *EnvConfig) PublicURL() string {
	if c.Hostname == "localhost" {
		return "http://localhost"
	}
	return fmt.Sprintf("https://%s", c.Hostname)
}

type EnvProxyConfig struct {
	AllowHTTP2       bool
	HeadersTimeout   int
	BodyTimeout      int
	MaxResponseSize  int
	MaxRetries       int
	PreferCompressed bool
}

type EnvEntrywayConfig struct {
	*ConfigService
	JWTVerifyKeyK256PublicKeyHex string
	PlcRotationKey               string
}

type EnvInviteConfig struct {
	Required bool
	Interval int
	Epoch    int
}

func d(v *string, deflt string) {
	if v == nil {
		return
	}
	if len(*v) == 0 {
		*v = deflt
	}
}
