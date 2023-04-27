package provider

import (
	"fmt"
	"math/big"

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

func ManifestFromDescriptor(desc *remote.Descriptor) (*Manifest, error) {
	switch {
	case desc.MediaType.IsImage():
		img, err := desc.Image()
		if err != nil {
			return nil, err
		}
		imf, err := img.Manifest()
		if err != nil {
			return nil, err
		}
		return &Manifest{
			SchemaVersion: basetypes.NewNumberValue(big.NewFloat(float64(imf.SchemaVersion))),
			MediaType:     basetypes.NewStringValue(string(imf.MediaType)),
			Config:        ToDescriptor(&imf.Config),
			Layers:        ToDescriptors(imf.Layers),
			Annotations:   ToStringMap(imf.Annotations),
			Subject:       ToDescriptor(imf.Subject),
			Manifests:     nil,
		}, nil
	case desc.MediaType.IsIndex():
		idx, err := desc.ImageIndex()
		if err != nil {
			return nil, err
		}
		imf, err := idx.IndexManifest()
		if err != nil {
			return nil, err
		}
		return &Manifest{
			SchemaVersion: basetypes.NewNumberValue(big.NewFloat(float64(imf.SchemaVersion))),
			MediaType:     basetypes.NewStringValue(string(imf.MediaType)),
			Manifests:     ToDescriptors(imf.Manifests),
			Annotations:   ToStringMap(imf.Annotations),
			Subject:       ToDescriptor(imf.Subject),
			Config:        nil,
			Layers:        nil,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported media type: %s", desc.MediaType)
	}
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
