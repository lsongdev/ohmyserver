package providers

import (
	"fmt"

	"github.com/lsongdev/ohmyserver/dnsprovider/providers/cloudflare"
	"github.com/lsongdev/ohmyserver/dnsprovider/providers/dnspod"
	"github.com/lsongdev/ohmyserver/dnsprovider/providers/namecom"
	"github.com/lsongdev/ohmyserver/dnsprovider/types"
)

func NewDNSProviderByName(name string, config types.Config) (types.IDNSProvider, error) {
	switch name {
	case "cloudflare":
		return cloudflare.NewDNSProvider(config)
	case "dnspod":
		return dnspod.NewDNSProvider(config)
	case "namecom":
		return namecom.NewDNSProvider(config)
	default:
		return nil, fmt.Errorf("unsupported DNS provider %q", name)
	}
}
