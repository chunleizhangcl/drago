package nic

import (
	"fmt"
	"math/rand"
	"encoding/hex"
	"regexp"	

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Settings :
type Settings struct {
	Name      	string
	Alias		*string	
	Address   	*netlink.Addr
	Wireguard	*wgtypes.Config
}

// NetworkInterface :
type NetworkInterface struct {
	Settings *Settings
	Link     *netlink.Link
}

// NetworkInterfaceCtrl :
type NetworkInterfaceCtrl struct {
	NetworkInterfaces map[string]*NetworkInterface

	namePrefix	 string
	wgController *wgctrl.Client
	wgPrivateKey *wgtypes.Key
}

// NewCtrl :
func NewCtrl(namePrefix string) (*NetworkInterfaceCtrl, error) {

	wg, err := wgctrl.New()
	if err != nil {
		return nil, err
	}

	pk, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}

	return &NetworkInterfaceCtrl{
		NetworkInterfaces: make(map[string]*NetworkInterface),
		wgController:   wg,
		wgPrivateKey:   &pk,
		namePrefix:		namePrefix,
	}, nil
}

// Update :
func (n *NetworkInterfaceCtrl) Update(ts []Settings) error {
	if err := n.resetWgNetworkInterfaces(); err != nil {
		return err
	}

	for _, s := range ts {
		b := make([]byte, 5) //equals 10 charachters
		rand.Read(b) 
		r := hex.EncodeToString(b)
		s.Name = n.namePrefix+r
		if err := n.ConfigureNetworkInterface(&s); err != nil {
			return err
		}
	}
	return nil
}

func (n *NetworkInterfaceCtrl) resetWgNetworkInterfaces() error {
	niList, _ := netlink.LinkList()
	for _, ni := range niList {
		if ni.Type() == "wireguard" {
			//match device alias with prefix provided by n.namePrefix 
			matched, err := regexp.MatchString(n.namePrefix+`.*`, ni.Attrs().Name)
			if err != nil {
				return fmt.Errorf("Failed to match interface name: %v\n", err)
			}
		
			if matched {
				if err := n.DeleteNetworkInterface(ni.Attrs().Name); err != nil {
					return fmt.Errorf("Failed to delete network interface: %v\n", err)
				}
				delete(n.NetworkInterfaces, ni.Attrs().Alias)
			}

		}

	}
	return nil
}

// ConfigureNetworkInterface :
func (n *NetworkInterfaceCtrl) ConfigureNetworkInterface(ts *Settings) error {
	// register new interface
	lattr := netlink.NewLinkAttrs()
	lattr.Name = ts.Name
	lattr.Alias = *ts.Alias

	if err := netlink.LinkAdd(&netlink.Wireguard{LinkAttrs: lattr}); err != nil {
		return fmt.Errorf("Failed to create new network device: %v\n", err)
	}

	l, err := netlink.LinkByName(ts.Name)
	if err != nil {
		return fmt.Errorf("Failed to get network device by name: %v\n", err)
	}

	if err := netlink.LinkSetAlias(l, lattr.Alias); err != nil {
		return fmt.Errorf("Failed to set link alias: %v\n", err)
	}


	// apply wireguard config
	if err := n.wgController.ConfigureDevice(ts.Name, *ts.Wireguard); err != nil {
		if err != nil {
			return fmt.Errorf("Unknown wireguard configuration error: %v\n", err)
		}
	}

	// apply new settings
	if err := netlink.AddrAdd(l, ts.Address); err != nil {
		return fmt.Errorf("Failed to add IP address: %v\n", err)
	}

	if err := netlink.LinkSetUp(l); err != nil {
		return fmt.Errorf("Failed to set network device up: %v\n", err)
	}

	for _, peer := range ts.Wireguard.Peers {
		for _, allowedIP := range peer.AllowedIPs {
			if err = netlink.RouteAdd(&netlink.Route{
				LinkIndex: l.Attrs().Index,
				Dst:       &allowedIP,
			}); err != nil {
				return fmt.Errorf("Failed to add IP route: %v\n", err)
			}
		}
	}

	n.NetworkInterfaces[*ts.Alias] = &NetworkInterface{
		Settings: ts,
		Link:     &l,
	}
	return nil
}

// DeleteNetworkInterface :
func (n *NetworkInterfaceCtrl) DeleteNetworkInterface(name string) error {
	lattr := netlink.NewLinkAttrs()
	lattr.Name = name

	ipRoutes, err := netlink.RouteList(&netlink.Wireguard{LinkAttrs: lattr}, 0)
	if err != nil {
		return fmt.Errorf("Failed to get IP routes list: %v\n", err)
	}
	for _, route := range ipRoutes {
		if err = netlink.RouteDel(&route); err != nil {
			return fmt.Errorf("Failed to remove IP route: %v\n", err)
		}
	}

	if err := netlink.LinkDel(&netlink.Wireguard{LinkAttrs: lattr}); err != nil {
		return fmt.Errorf("Failed to delete network device: %v\n", err)
	}

	return nil
}

// GetWgPrivateKey :
func (n *NetworkInterfaceCtrl) GetWgPrivateKey() *wgtypes.Key {
	return n.wgPrivateKey
}
