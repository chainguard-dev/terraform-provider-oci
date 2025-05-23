package structure

// verifyEnv contains a best-effort list of PATH-esque environment variables
// which should be evaluated for literal `$VAR` string usage.
var verifyEnv = map[string]string{
	"CDC_AGENT_PATH":       ":",
	"CRI_CONFIG_PATH":      ":",
	"GATUS_CONFIG_PATH":    ":",
	"GCONV_PATH":           ":",
	"GEM_PATH":             ":",
	"GETCONF_DIR":          ":",
	"GOPATH":               ":",
	"JAVA_HOME":            ":",
	"KO_DATA_PATH":         ":",
	"LD_LIBRARY_PATH":      ":",
	"LD_ORIGIN_PATH":       ":",
	"LD_PRELOAD":           ":",
	"LIBRARY_PATH":         ":",
	"LOCPATH":              ":",
	"LUA_CPATH":            ";", // the LUA path delimiter is a semicolon (https://www.lua.org/pil/8.1.html)
	"LUA_PATH":             ";", // the LUA path delimiter is a semicolon (https://www.lua.org/pil/8.1.html)
	"MAAC_PATH":            ":",
	"MCAC_PATH":            ":",
	"NKEYS_PATH":           ":",
	"NIS_PATH":             ":",
	"NLSPATH":              ":",
	"OPENSEARCH_PATH_CONF": ":",
	"PATH":                 ":",
	"PERLLIB":              ":",
	"PYTHONPATH":           ":",
	"RESOLV_HOST_CONF":     ":",
	"TMPDIR":               ":",
	"TZDIR":                ":",
	"ZAP_PATH":             ":",
}
