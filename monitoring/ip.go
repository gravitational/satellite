package monitoring

// NewIPForwardChecker returns new IP forward checker
func NewIPForwardChecker() *SysctlChecker {
	return &SysctlChecker{
		CheckerName:     "ip-forward",
		Param:           "net.ipv4.ip_forward",
		Expected:        "1",
		OnMissing:       "ipv4 forwarding status unknown",
		OnValueMismatch: "ipv4 forwarding off",
	}
}

func NewBrNetfilterChecker() *SysctlChecker {
	return &SysctlChecker{
		CheckerName:     "br-netfilter",
		Param:           "net.bridge.bridge-nf-call-iptables",
		Expected:        "1",
		OnMissing:       "br_netfilter module is possibly not loaded",
		OnValueMismatch: "kubernetes requires net.bridge.bridge-nf-call-iptables sysctl set to 1",
	}
}
