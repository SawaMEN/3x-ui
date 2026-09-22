package service

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	telemtWebStatePath  = "/etc/x-ui/telemt-web.json"
	telemtWebNginxConf  = "/etc/nginx/conf.d/3x-ui-telemt-web.conf"
	telemtWebAcmeConf   = "/etc/nginx/conf.d/3x-ui-telemt-web-acme.conf"
	telemtWebDecoyDir   = "/var/lib/x-ui/telemt-web"
	telemtWebListenIP   = "127.0.0.1"
	telemtWebListenPort = 15080
	telemtWebUser       = "webproxy"
	telemtWebMinEngine  = "3.5.1"
)

var telemtWebDomainPattern = regexp.MustCompile(
	`^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$`,
)