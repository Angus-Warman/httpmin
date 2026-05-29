package httpmin

import (
	"fmt"
	"net"
)

func printAddresses(protocol, ip, port string) {
	addresses := getAddresses(protocol, ip, port)

	if len(addresses) == 1 {
		fmt.Printf("Listening on %v\n", addresses[0])
		return
	}

	fmt.Println("Listening on:")

	for _, address := range addresses {
		fmt.Printf("  %v\n", address)
	}
}

func getAddresses(protocol, ip, port string) []string {
	addresses := []string{}

	if ip != "0.0.0.0" {
		addresses = append(addresses, fmt.Sprintf("%v://%v:%v", protocol, ip, port))
		return addresses
	}

	ifaces, err := net.Interfaces()

	if err != nil {
		addresses = append(addresses, fmt.Sprintf("%v://%v:%v", protocol, ip, port))
		return addresses
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
				continue
			}
			addresses = append(addresses, fmt.Sprintf("%v://%v:%v", protocol, ip, port))
		}
	}

	return addresses
}
