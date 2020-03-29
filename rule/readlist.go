package rule

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"math"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/LincolnYe/avege/common"
	"github.com/LincolnYe/avege/common/fs"
	"github.com/LincolnYe/avege/config"
)

func getExceptionDomainList() (res []string) {
	exceptionFile, err := fs.GetConfigPath(`exception.txt`)
	if err != nil {
		exceptionFile = `exception.txt`
	}
	inFile, err := os.Open(exceptionFile)
	if err != nil {
		common.Error("opening exception.txt failed", err)
		return
	}
	defer inFile.Close()
	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		rec := scanner.Text()
		res = append(res, rec)
	}
	return
}

func resolveIPFromDomainName(host string) (res []string) {
	if ips, err := net.LookupIP(host); err == nil {
		for _, ip := range ips {
			if ip.To4() != nil {
				res = append(res, ip.String())
			}
		}
	}
	return
}

func addProxyServerIPs(encountered map[string]placeholder) (records []string) {
	// ss servers ip
	record := "add cnroute %s/32"
	var ss []string
	// SSR subscription
	if config.Configurations.Generals.SSRSubscriptionEnabled {
		res := getSSRSubcription()
		if len(res) == 0 {
			// read from history
			if file, err := os.OpenFile(`ssrsub.history`, os.O_RDONLY, 0644); err != nil {
				common.Error("open ssrsub.history file for read failed", err)
			} else {
				r, err := ioutil.ReadAll(file)
				if err != nil {
					common.Error("reading ssrsub.history failed", err)
				} else {
					res = strings.Split(string(r), "\n")
				}
				file.Close()
				common.Debug("ssrsub.history has been read")
			}
		}
		ss = append(ss, res...)
		// save to history
		if file, err := os.OpenFile(`ssrsub.history`, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644); err != nil {
			common.Error("open ssrsub.history file failed", err)
		} else {
			file.WriteString(strings.Join(res, "\n"))
			file.Close()
			common.Debug("ssrsub.history file has been updated")
		}
	}
	// exception domains
	exception := getExceptionDomainList()
	// resolve
	dl := append(ss, exception...)
	for _, host := range dl {
		ips := resolveIPFromDomainName(host)
		for _, v := range ips {
			val := fmt.Sprintf(record, v)
			if _, ok := encountered[val]; !ok {
				// don't insert duplicated items
				encountered[val] = placeholder{}
				records = append(records, val)
			}
		}
	}
	return
}

func filterSpecialIPs(encountered map[string]placeholder) (records []string, recordMap map[string][]string) {
	apnicFile, err := fs.GetConfigPath("apnic.txt")
	if err != nil {
		apnicFile = "apnic.txt"
	}
	inFile, err := os.Open(apnicFile)
	if err != nil {
		common.Error("opening apnic.txt failed", err)
		return
	}
	defer inFile.Close()
	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)

	recordMap = make(map[string][]string)
	for scanner.Scan() {
		rec := scanner.Text()
		s := strings.Split(rec, "|")
		if len(s) == 7 && strings.ToLower(s[0]) == "apnic" && strings.ToLower(s[2]) == "ipv4" {
			prefix := strings.ToLower(s[1])
			v, err := strconv.ParseFloat(s[4], 64)
			if err != nil {
				common.Errorf("converting string %s to float64 failed\n", s[4])
				continue
			}
			mask := 32 - math.Log2(v)
			if prefix == "cn" {
				// china IPs
				records = append(records, fmt.Sprintf("add cnroute %s/%d", s[3], int(mask)))
			} else if prefixLocalPortMap.Contains(prefix) {
				rs, ok := recordMap[prefix]
				if ok {
					rs = append(rs, fmt.Sprintf("add %sroute %s/%d", prefix, s[3], int(mask)))
				} else {
					rs = []string{fmt.Sprintf("add %sroute %s/%d", prefix, s[3], int(mask))}
				}
				recordMap[prefix] = rs
			} else {
				// skip
			}
		}
	}

	return
}

func addCurrentRunningServerIPs(encountered map[string]placeholder) (res []string) {
	// current running server addresses
	record := "add cnroute %s/32"
	for _, outbound := range config.Configurations.OutboundsConfig {
		host, _, _ := net.SplitHostPort(outbound.Address)
		ips, err := net.LookupIP(host)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if ip.To4() == nil {
				// invalid IPv4 address
				continue
			}
			val := fmt.Sprintf(record, ip.String())
			if _, ok := encountered[val]; ok {
				// don't insert duplicated items
				continue
			}
			encountered[val] = placeholder{}
			res = append(res, val)
		}
	}
	return
}
