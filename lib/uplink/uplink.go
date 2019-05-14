// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package uplink

import (
	"context"

	"github.com/vivint/infectious"

	"storj.io/storj/internal/memory"
	"storj.io/storj/pkg/eestream"
	"storj.io/storj/pkg/identity"
	"storj.io/storj/pkg/metainfo/kvmetainfo"
	"storj.io/storj/pkg/peertls/tlsopts"
	"storj.io/storj/pkg/storage/buckets"
	"storj.io/storj/pkg/storage/segments"
	"storj.io/storj/pkg/storage/streams"
	"storj.io/storj/pkg/storj"
	"storj.io/storj/pkg/transport"
	"storj.io/storj/uplink/metainfo"
)

var (
	maxBucketMetaSize = 10 * memory.MiB
)

// Config represents configuration options for an Uplink
type Config struct {
	// Volatile groups config values that are likely to change semantics
	// or go away entirely between releases. Be careful when using them!
	Volatile struct {
		// TLS defines options that affect TLS negotiation for outbound
		// connections initiated by this uplink.
		TLS struct {
			// SkipPeerCAWhitelist determines whether to require all
			// remote hosts to have identity certificates signed by
			// Certificate Authorities in the default whitelist. If
			// set to true, the whitelist will be ignored.
			SkipPeerCAWhitelist bool

			// PeerCAWhitelistPath gives the path to a CA cert
			// whitelist file. It is ignored if SkipPeerCAWhitelist
			// is set. If empty, the internal default peer whitelist
			// is used.
			PeerCAWhitelistPath string
		}

		// PeerIDVersion is the identity versions remote peers to this node
		// will be supported by this node.
		PeerIDVersion string

		// MaxInlineSize determines whether the uplink will attempt to
		// store a new object in the satellite's metainfo. Objects at
		// or below this size will be marked for inline storage, and
		// objects above this size will not. (The satellite may reject
		// the inline storage and require remote storage, still.)
		MaxInlineSize memory.Size

		// MaxMemory is the default maximum amount of memory to be
		// allocated for read buffers while performing decodes of
		// objects. (This option is overrideable per Bucket if the user
		// so desires.) If set to zero, the library default (4 MiB) will
		// be used. If set to a negative value, the system will use the
		// smallest amount of memory it can.
		MaxMemory memory.Size
	}
}

func (cfg *Config) clone() *Config {
	clone := *cfg
	return &clone
}

func (cfg *Config) setDefaults(ctx context.Context) error {
	if cfg.Volatile.MaxInlineSize == 0 {
		cfg.Volatile.MaxInlineSize = 4 * memory.KiB
	}
	if cfg.Volatile.MaxMemory.Int() == 0 {
		cfg.Volatile.MaxMemory = 4 * memory.MiB
	} else if cfg.Volatile.MaxMemory.Int() < 0 {
		cfg.Volatile.MaxMemory = 0
	}
	return nil
}

// Uplink represents the main entrypoint to Storj V3. An Uplink connects to
// a specific Satellite and caches connections and resources, allowing one to
// create sessions delineated by specific access controls.
type Uplink struct {
	ident *identity.FullIdentity
	tc    transport.Client
	cfg   *Config
}

// NewUplink creates a new Uplink. This is the first step to create an uplink
// session with a user specified config or with default config, if nil config
func NewUplink(ctx context.Context, cfg *Config) (*Uplink, error) {
	ident, err := identity.NewFullIdentity(ctx, identity.NewCAOptions{
		Difficulty:  0,
		Concurrency: 1,
	})
	if err != nil {
		return nil, err
	}

	if cfg == nil {
		cfg = &Config{}
	}
	cfg = cfg.clone()
	if err := cfg.setDefaults(ctx); err != nil {
		return nil, err
	}
	tlsConfig := tlsopts.Config{
		UsePeerCAWhitelist:  !cfg.Volatile.TLS.SkipPeerCAWhitelist,
		PeerCAWhitelistPath: cfg.Volatile.TLS.PeerCAWhitelistPath,
		PeerIDVersions:      "0",
	}
	tlsOpts, err := tlsopts.NewOptions(ident, tlsConfig)
	if err != nil {
		return nil, err
	}
	tc := transport.NewClient(tlsOpts)

	return &Uplink{
		ident: ident,
		tc:    tc,
		cfg:   cfg,
	}, nil
}

// ProjectOptions allows configuration of various project options during opening
type ProjectOptions struct {
	Volatile struct {
		EncryptionKey *storj.Key
	}
}

// OpenProject returns a Project handle with the given APIKey
func (u *Uplink) OpenProject(ctx context.Context, satelliteAddr string, apiKey APIKey, opts *ProjectOptions) (p *Project, err error) {
	defer mon.Task()(&ctx)(&err)

	metainfo, err := metainfo.NewClient(ctx, u.tc, satelliteAddr, apiKey.Serialize())
	if err != nil {
		return nil, err
	}

	// TODO: we shouldn't really need encoding parameters to manage buckets.
	whoCares := 1
	fc, err := infectious.NewFEC(whoCares, whoCares)
	if err != nil {
		return nil, Error.New("failed to create erasure coding client: %v", err)
	}
	rs, err := eestream.NewRedundancyStrategy(eestream.NewRSScheme(fc, whoCares), whoCares, whoCares)
	if err != nil {
		return nil, Error.New("failed to create redundancy strategy: %v", err)
	}
	segments := segments.NewSegmentStore(metainfo, nil, rs, maxBucketMetaSize.Int(), maxBucketMetaSize.Int64())
	var encryptionKey *storj.Key
	if opts != nil {
		encryptionKey = opts.Volatile.EncryptionKey
	}
	if encryptionKey == nil {
		// volatile warning: we're setting an encryption key of all zeros when one isn't provided.
		// TODO: fix before the final alpha network wipe
		encryptionKey = new(storj.Key)
	}
	streams, err := streams.NewStreamStore(segments, maxBucketMetaSize.Int64(),
		encryptionKey, memory.KiB.Int(), storj.AESGCM)
	if err != nil {
		return nil, Error.New("failed to create stream store: %v", err)
	}

	return &Project{
		uplinkCfg:     u.cfg,
		tc:            u.tc,
		metainfo:      metainfo,
		project:       kvmetainfo.NewProject(buckets.NewStore(streams), memory.KiB.Int32(), rs, 64*memory.MiB.Int64()),
		maxInlineSize: u.cfg.Volatile.MaxInlineSize,
		encryptionKey: encryptionKey,
	}, nil
}

// Close closes the Uplink. This may not do anything at present, but should
// still be called to allow forward compatibility. No Project or Bucket
// objects using this Uplink should be used after calling Close.
func (u *Uplink) Close() error {
	return nil
}
