package sub

import (
	"crypto/rand"
	"math/big"
	"net"
	"net/netip"
	"strings"
)

const MaxRandomCIDRCount = 100

func cleanCIDR(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return s
	}
	address, rest, ok := strings.Cut(s[1:], "]")
	if ok && (rest == "" || strings.HasPrefix(rest, "/")) {
		return address + rest
	}
	return s
}

// IsCIDR reports whether s represents a valid CIDR network notation.
func IsCIDR(s string) bool {
	_, _, err := net.ParseCIDR(cleanCIDR(s))
	return err == nil
}

// RandomIPsFromCIDR returns count unique random host IPs from the specified CIDR.
// Supported across IPv4 and IPv6; caps at the usable address space when smaller.
func RandomIPsFromCIDR(cidr string, count int) ([]string, error) {
	count = min(max(count, 1), MaxRandomCIDRCount)
	ip, network, err := net.ParseCIDR(cleanCIDR(cidr))
	if err != nil {
		return nil, err
	}
	prefixBits, addressBits := network.Mask.Size()
	addr := netip.AddrFrom16([16]byte(ip))
	if addressBits == 32 {
		addr = addr.Unmap()
	}
	prefix, err := addr.Prefix(prefixBits)
	if err != nil {
		return nil, err
	}
	addr = prefix.Addr()
	hostBits := addressBits - prefixBits
	if hostBits == 0 {
		return []string{addr.String()}, nil
	}
	if hostBits == 1 {
		result := []string{addr.String()}
		if count > 1 {
			result = append(result, addr.Next().String())
		}
		return result, nil
	}

	// Exclude the network/anycast address and, for IPv4, the broadcast address.
	one := big.NewInt(1)
	usable := new(big.Int).Lsh(one, uint(hostBits))
	usable.Sub(usable, one)
	if addr.Is4() {
		usable.Sub(usable, one)
	}
	if usable.Cmp(big.NewInt(int64(count))) < 0 {
		count = int(usable.Int64())
	}

	addressBytes := addr.AsSlice()
	base := new(big.Int).SetBytes(addressBytes)
	result := make([]string, 0, count)
	replacements := make(map[string]*big.Int, count)

	// Draw from the remaining offsets as in a partial Fisher-Yates shuffle.
	// Only moved offsets are stored, even for very large IPv6 networks.
	for range count {
		offset, err := rand.Int(rand.Reader, usable)
		if err != nil {
			return nil, err
		}
		offset.Add(offset, one)
		key := string(offset.Bytes())
		if replacement := replacements[key]; replacement != nil {
			offset.Set(replacement)
		}
		lastKey := string(usable.Bytes())
		if replacement := replacements[lastKey]; replacement != nil {
			replacements[key] = replacement
		} else {
			replacements[key] = new(big.Int).Set(usable)
		}
		delete(replacements, lastKey)
		usable.Sub(usable, one)

		offset.Add(offset, base).FillBytes(addressBytes)
		// The slice retains the prefix's address family, including mapped IPv6.
		ip, _ := netip.AddrFromSlice(addressBytes)
		result = append(result, ip.String())
	}
	return result, nil
}
