package utilities

import (
	"bytes"
	"net"
)

func RetrievePhysicalMacAddr() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var as []string
	for _, ifa := range interfaces {
		if ifa.Flags&net.FlagUp != 0 && bytes.Compare(ifa.HardwareAddr, nil) != 0 {
			if ifa.HardwareAddr[0]&2 == 2 {
				continue
			}
			a := ifa.HardwareAddr.String()
			if a != "" {
				as = append(as, a)
			}
		}

	}
	return as, nil
}

func RetrieveMacAddr() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var as []string
	for _, ifa := range interfaces {
		a := ifa.HardwareAddr.String()
		if a != "" {
			as = append(as, a)
		}
	}
	return as, nil
}

func GetOutboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer func() {
		cErr := conn.Close()
		if cErr != nil && err == nil {
			err = cErr
		}
	}()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, nil
}
