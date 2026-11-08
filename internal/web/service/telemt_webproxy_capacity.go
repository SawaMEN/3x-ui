package service

// telemtWebProfileCapacity reserves bounded headroom for the next users and
// aliases. Capacity changes require a Telemt restart, so a geometric growth
// policy avoids a restart for every new client in a populated WEB inbound.
func telemtWebProfileCapacity(users, hosts int) int {
	if users < 0 {
		users = 0
	}
	if hosts < 1 {
		hosts = 1
	}
	needed := (users + 16) * hosts
	capacity := 64
	for capacity < needed && capacity < 1<<20 {
		capacity *= 2
	}
	if capacity < needed {
		return needed
	}
	return capacity
}
