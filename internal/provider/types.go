package provider

import (
	"fmt"
	"math/big"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

type Image struct {
	Digest   types.String `tfsdk:"digest"`
	ImageRef types.String `tfsdk:"image_ref"`
}

type Manifest struct {
	SchemaVersion types.Number            `tfsdk:"schema_version"`
	MediaType     types.String            `tfsdk:"media_type"`
	Config        *Descriptor             `tfsdk:"config"`
	Layers        []Descriptor            `tfsdk:"layers"`
	Annotations   map[string]types.String `tfsdk:"annotations"`
	Manifests     []Descriptor            `tfsdk:"manifests"`
	Subject       *Descriptor             `tfsdk:"subject"`
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
		m.SchemaVersion = basetypes.NewNumberValue(big.NewFloat(float64(imf.SchemaVersion)))
		m.MediaType = basetypes.NewStringValue(string(imf.MediaType))
		m.Config = ToDescriptor(&imf.Config)
		m.Layers = ToDescriptors(imf.Layers)
		m.Annotations = ToStringMap(imf.Annotations)
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
		m.SchemaVersion = basetypes.NewNumberValue(big.NewFloat(float64(imf.SchemaVersion)))
		m.MediaType = basetypes.NewStringValue(string(imf.MediaType))
		m.Manifests = ToDescriptors(imf.Manifests)
		m.Annotations = ToStringMap(imf.Annotations)
		m.Subject = ToDescriptor(imf.Subject)
		m.Config = nil
		m.Layers = nil
		return nil
	}

	return fmt.Errorf("unsupported media type: %s", desc.MediaType)
}

func ToStringMap(m map[string]string) map[string]basetypes.StringValue {
	if m == nil {
		return map[string]basetypes.StringValue{}
	}
	out := make(map[string]basetypes.StringValue, len(m))
	for k, v := range m {
		out[k] = basetypes.NewStringValue(v)
	}
	return out
}

func ToDescriptor(d *v1.Descriptor) *Descriptor {
	if d == nil {
		return nil
	}
	return &Descriptor{
		MediaType: basetypes.NewStringValue(string(d.MediaType)),
		Size:      basetypes.NewNumberValue(big.NewFloat(float64(d.Size))),
		Digest:    basetypes.NewStringValue(d.Digest.String()),
		Platform:  ToPlatform(d.Platform),
	}
}

func ToPlatform(p *v1.Platform) *Platform {
	if p == nil {
		return nil
	}
	return &Platform{
		Architecture: basetypes.NewStringValue(p.Architecture),
		OS:           basetypes.NewStringValue(p.OS),
		Variant:      basetypes.NewStringValue(p.Variant),
		OSVersion:    basetypes.NewStringValue(p.OSVersion),
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
	MediaType types.String `tfsdk:"media_type"`
	Size      types.Number `tfsdk:"size"`
	Digest    types.String `tfsdk:"digest"`
	Platform  *Platform    `tfsdk:"platform"`
}

type Platform struct {
	Architecture types.String `tfsdk:"architecture"`
	OS           types.String `tfsdk:"os"`
	Variant      types.String `tfsdk:"variant"`
	OSVersion    types.String `tfsdk:"os_version"`
}

type Config struct {
	Env        []types.String `tfsdk:"env"`
	User       types.String   `tfsdk:"user"`
	WorkingDir types.String   `tfsdk:"working_dir"`
	Entrypoint []types.String `tfsdk:"entrypoint"`
	Cmd        []types.String `tfsdk:"cmd"`
	CreatedAt  types.String   `tfsdk:"created_at"`
}

func (c *Config) FromConfigFile(cf *v1.ConfigFile) {
	if c == nil {
		c = &Config{}
	}
	if cf == nil {
		return
	}

	c.Env = ToStrings(cf.Config.Env)
	c.User = basetypes.NewStringValue(cf.Config.User)
	c.WorkingDir = basetypes.NewStringValue(cf.Config.WorkingDir)
	c.Entrypoint = ToStrings(cf.Config.Entrypoint)
	c.Cmd = ToStrings(cf.Config.Cmd)
	c.CreatedAt = basetypes.NewStringValue(cf.Created.Time.Format(time.RFC3339))
}

func ToStrings(ss []string) []basetypes.StringValue {
	if len(ss) == 0 {
		return nil
	}
	out := make([]basetypes.StringValue, len(ss))
	for i, s := range ss {
		out[i] = basetypes.NewStringValue(s)
	}
	return out
}
