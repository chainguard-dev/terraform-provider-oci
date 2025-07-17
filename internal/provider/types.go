package provider

import (
	"fmt"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type Image struct {
	Digest   string `tfsdk:"digest"`
	ImageRef string `tfsdk:"image_ref"`
}

type Manifest struct {
	SchemaVersion int64             `tfsdk:"schema_version"`
	MediaType     string            `tfsdk:"media_type"`
	Config        *Descriptor       `tfsdk:"config"`
	Layers        []Descriptor      `tfsdk:"layers"`
	Annotations   map[string]string `tfsdk:"annotations"`
	Manifests     []Descriptor      `tfsdk:"manifests"`
	Subject       *Descriptor       `tfsdk:"subject"`
}

func (m *Manifest) FromDescriptor(desc *remote.Descriptor) error {
	switch {
	case desc.MediaType.IsImage():
		img, err := desc.Image()
		if err != nil {
			return err
		}
		imf, err := img.Manifest()
		if err != nil {
			return err
		}
		m.SchemaVersion = imf.SchemaVersion
		m.MediaType = string(imf.MediaType)
		m.Config = ToDescriptor(&imf.Config)
		m.Layers = ToDescriptors(imf.Layers)
		m.Annotations = imf.Annotations
		m.Subject = ToDescriptor(imf.Subject)
		m.Manifests = nil
		return nil

	case desc.MediaType.IsIndex():
		idx, err := desc.ImageIndex()
		if err != nil {
			return err
		}
		imf, err := idx.IndexManifest()
		if err != nil {
			return err
		}
		m.SchemaVersion = imf.SchemaVersion
		m.MediaType = string(imf.MediaType)
		m.Manifests = ToDescriptors(imf.Manifests)
		m.Annotations = imf.Annotations
		m.Subject = ToDescriptor(imf.Subject)
		m.Config = nil
		m.Layers = nil
		return nil
	}

	return fmt.Errorf("unsupported media type: %s", desc.MediaType)
}

func ToDescriptor(d *v1.Descriptor) *Descriptor {
	if d == nil {
		return nil
	}
	return &Descriptor{
		MediaType: string(d.MediaType),
		Size:      d.Size,
		Digest:    d.Digest.String(),
		Platform:  ToPlatform(d.Platform),
	}
}

func ToPlatform(p *v1.Platform) *Platform {
	if p == nil {
		return nil
	}
	return &Platform{
		Architecture: p.Architecture,
		OS:           p.OS,
		Variant:      p.Variant,
		OSVersion:    p.OSVersion,
	}
}

func ToDescriptors(d []v1.Descriptor) []Descriptor {
	out := make([]Descriptor, len(d))
	for i, desc := range d {
		out[i] = *ToDescriptor(&desc)
	}
	return out
}

type Descriptor struct {
	MediaType string    `tfsdk:"media_type"`
	Size      int64     `tfsdk:"size"`
	Digest    string    `tfsdk:"digest"`
	Platform  *Platform `tfsdk:"platform"`
}

type Platform struct {
	Architecture string `tfsdk:"architecture"`
	OS           string `tfsdk:"os"`
	Variant      string `tfsdk:"variant"`
	OSVersion    string `tfsdk:"os_version"`
}

type Config struct {
	Env        []string `tfsdk:"env"`
	User       string   `tfsdk:"user"`
	WorkingDir string   `tfsdk:"working_dir"`
	Entrypoint []string `tfsdk:"entrypoint"`
	Cmd        []string `tfsdk:"cmd"`
	CreatedAt  string   `tfsdk:"created_at"`
}

func (c *Config) FromConfigFile(cf *v1.ConfigFile) {
	if c == nil {
		c = &Config{}
	}
	if cf == nil {
		return
	}

	c.Env = cf.Config.Env
	c.User = cf.Config.User
	c.WorkingDir = cf.Config.WorkingDir
	c.Entrypoint = cf.Config.Entrypoint
	c.Cmd = cf.Config.Cmd
	c.CreatedAt = cf.Created.Format(time.RFC3339)
}
