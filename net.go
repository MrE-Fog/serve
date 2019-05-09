package main

import (
	"log"
	"net"
	"os"
	"runtime"
	"strings"
)

// getAddressesFromIface goes through the addresses of the given interface and tries to return the first of each kind.
//
// The interesting interfaces like eth0 and wlan0 typically have 2 addresses: one IPv4 and one IPv6 address.
// But some interfaces just have one of them, or if an interface is deactivated it doesn't have any.
// On Windows the main network interface like "Ethernet 3" can have many addresses and the main IPv4 address doesn't have to be one of the first 2.
// We must take care of all these combinations.
func getAddressesFromIface(iface net.Interface) (ipv4 string, ipv6 string) {
	addrs, err := iface.Addrs()
	if err != nil {
		log.Fatal(err)
	}
	for i := 0; i < len(addrs) && (ipv4 == "" || ipv6 == ""); i++ {
		// In the case of two addresses they could potentially be of the same type.
		// We want to show the first address. overwriteIfEmpty() doesn't overwrite existing values.
		addrWithoutMask := strings.Split(addrs[i].String(), "/")[0]
		if strings.Contains(addrWithoutMask, ":") {
			overwriteIfEmpty(&ipv4, "")
			overwriteIfEmpty(&ipv6, addrWithoutMask)
		} else {
			overwriteIfEmpty(&ipv4, addrWithoutMask)
			overwriteIfEmpty(&ipv6, "")
		}
	}
	return
}

// isFav checks the network interface's name and if it's a typical main one (like "eth0" on Linux) it returns true.
//
// Note: All possible runtime.GOOS values are listed here: https://golang.org/doc/install/source#environment
func isFav(iface net.Interface) bool {
	switch runtime.GOOS {
	case "windows":
		if iface.Name == "WiFi" ||
			len(iface.Name) >= 8 && iface.Name[:8] == "Ethernet" {
			return true
		}
	case "darwin":
		if iface.Name == "en0" || iface.Name == "en1" {
			return true
		}
	case "linux":
		if iface.Name == "eth0" || iface.Name == "wlan0" {
			return true
		}
	}
	return false
}

// defaultSANs returns DNS names and IP addresses that might be used to reach the current host,
// either from the host itself or from other machines in the local network.
func defaultSANs() []string {
	result := []string{"localhost", "127.0.0.1"}

	hostname, err := os.Hostname()
	if err == nil {
		result = append(result, hostname, hostname+".local", "*."+hostname+".local", hostname+".lan", "*."+hostname+".lan", hostname+".home", "*."+hostname+".home")
	}

	lanIP, err := lanIP()
	if err == nil {
		result = append(result, lanIP)
	}

	return result
}

// lanIP tries to determine the IP address of the current machine in the LAN.
func lanIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	fav := ""
	for _, iface := range ifaces {
		if isFav(iface) {
			fav, _ = getAddressesFromIface(iface)
			break
		}
	}
	return fav, nil
}

type statusWriter struct {
	http.ResponseWriter
	status int
	length int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	n, err := w.ResponseWriter.Write(b)
	w.length += n
	return n, err
}

func withTracing(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(response, r)
		defer log.Printf("%s [%s] %q %s %d %d %q", r.RemoteAddr, r.Method, r.RequestURI, r.Proto, response.status, response.length, r.Header.Get("User-Agent"))
	}
}

func recoverHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %+v", err)
				http.Error(w, http.StatusText(500), 500)
			}
		}()

		next.ServeHTTP(w, r)
	}
}
