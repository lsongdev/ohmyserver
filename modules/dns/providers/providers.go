package providers

import (
	"fmt"

	"github.com/lsongdev/ohmyserver/modules/dns/providers/cloudflare"
	"github.com/lsongdev/ohmyserver/modules/dns/providers/dnspod"
	"github.com/lsongdev/ohmyserver/modules/dns/providers/namecom"
	types "github.com/lsongdev/ohmyserver/modules/dns/providers/types"
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
