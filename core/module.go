package core

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	com "github.com/Wariie/go-woxy/com"
	auth "github.com/abbot/go-http-auth"
	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/process"
)

/*ModuleConfig - Module configuration */
type ModuleConfig struct {
	NAME    string
	VERSION int
	TYPES   string
	EXE     ModuleExecConfig
	BINDING ServerConfig
	STATE   ModuleState
	PK      string
	AUTH    ModuleAuthConfig
	pid     int
}

//GetServer - Get Module Server configuration
func (mc *ModuleConfig) GetServer(path string) com.Server {
	if path == "" {
		path = mc.BINDING.PATH[0].FROM
	}
	return com.Server{IP: mc.BINDING.ADDRESS, Path: path, Port: mc.BINDING.PORT, Protocol: mc.BINDING.PROTOCOL}
}

//Stop - Stop Module
func (mc *ModuleConfig) Stop() int {
	if mc.STATE != Online {
		return -1
	}
	var cr com.CommandRequest
	cr.Generate("Shutdown", mc.PK, mc.NAME, secretHash)
	r, err := com.SendRequest(mc.GetServer(""), &cr, false)
	log.Println("SHUTDOWN RESULT : ", r, err)
	//TODO BEST GESTURE
	if true {
		mc.STATE = Stopped
	}
	return 0
}

//GetLog - GetLog from Module
func (mc *ModuleConfig) GetLog() string {

	file, err := os.Open("./mods/" + mc.NAME + "/log.log")
	if err != nil {
		log.Panicf("failed reading file: %s", err)
	}
	b, err := ioutil.ReadAll(file)
	return string(b)
}

//GetPerf - GetPerf from Module
func (mc *ModuleConfig) GetPerf() (float64, float32) {
	p, err := process.NewProcess(int32(mc.pid))
	//sysinfo, err := pidusage.GetStat(mc.pid)
	log.Println(p, err)
	ram, err := p.MemoryPercent()
	log.Println(ram, err)
	cpu, err := p.Percent(0)
	log.Println(cpu, err)
	name, err := p.Name()
	log.Println(name, err)

	return cpu, ram
}

//Setup - Setup module from config
func (mc *ModuleConfig) Setup(router *gin.Engine, hook bool) error {
	fmt.Println("Setup mod : ", mc)
	if !reflect.DeepEqual(mc.EXE, ModuleExecConfig{}) {
		if strings.Contains(mc.EXE.SRC, "http") || strings.Contains(mc.EXE.SRC, "git@") {
			mc.Download()
		}
		mc.copySecret()
		go mc.Start()
	} // ELSE NO BUILD

	if hook {
		return mc.HookAll(router)
	}
	return nil
}

//Start - Start module with config args and auto args
func (mc *ModuleConfig) Start() {
	mc.STATE = Loading
	//logFileName := mc.NAME + ".txt"
	var platformParam []string
	if runtime.GOOS == "windows" {
		platformParam = []string{"cmd", "/c ", "go", "run", mc.EXE.MAIN, "1>", "log.log", "2>&1"}
	} else {
		platformParam = []string{"/bin/sh", "-c", "go run " + mc.EXE.MAIN + " > log.log 2>&1"}
	}

	fmt.Println("Starting mod : ", mc)
	cmd := exec.Command(platformParam[0], platformParam[1:]...)
	cmd.Dir = mc.EXE.BIN
	output, err := cmd.Output()
	if err != nil {
		log.Println("Error:", err)
	}
	log.Println("Output :", string(output), err)
}

func (mc *ModuleConfig) copySecret() {
	source, err := os.Open(".secret")
	if err != nil {
		log.Println("Error reading generated secret file")
	}
	defer source.Close()

	destination, err := os.Create(mc.EXE.BIN + "/.secret")
	if err != nil {
		log.Println("Error creating mod secret file")
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	if err != nil {
		log.Println("Error Copy Secret:", err, nBytes)
	}
}

//Download - Download module from repository ( git clone )
func (mc *ModuleConfig) Download() {

	if mc.STATE != Online {

		var listArgs []string
		var action string

		wd := "./mods/"
		if _, err := os.Stat(wd + mc.NAME + "/"); os.IsNotExist(err) {
			listArgs = []string{"clone", mc.EXE.SRC}
			action = "Downloaded"
		} else {
			listArgs = []string{"pull"}
			action = "Update"
			wd += mc.NAME + "/"
		}

		cmd := exec.Command("git", listArgs...)
		cmd.Dir = wd
		out, err := cmd.CombinedOutput()
		fmt.Println(action, " mod : ", mc, " - ", string(out), " ", err)

		mc.EXE.BIN = "./mods/" + mc.NAME + "/"
		mc.STATE = Downloaded
	} else {
		log.Println("Error - Trying to download/update module while running\nStop it before")
	}
}

//HookAll - Create all binding between module config address and gin server
func (mc *ModuleConfig) HookAll(router *gin.Engine) error {
	paths := mc.BINDING.PATH

	if strings.Contains(mc.TYPES, "web") {
		sP := ""
		if len(paths[0].FROM) > 1 {
			sP = paths[0].FROM
		}
		r := Route{FROM: sP + "/ressources/*filepath", TO: "/ressources/*filepath"}
		mc.Hook(router, r, "GET")
	}

	if len(paths) > 0 && len(paths[0].FROM) > 0 {
		for i := range paths {
			err := mc.Hook(router, paths[i], "GET")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

//Hook - Create a binding between module and gin server
func (mc *ModuleConfig) Hook(router *gin.Engine, r Route, typeR string) error {
	if typeR == "" {
		typeR = "GET"
	}
	if len(r.FROM) > 0 {
		if mc.AUTH.ENABLED {
			htpasswd := auth.HtpasswdFileProvider(".htpasswd")
			authenticator := auth.NewBasicAuthenticator("Some Realm", htpasswd)
			authorized := router.Group("/", BasicAuth(authenticator))
			authorized.Handle("GET", r.FROM, ReverseProxy(mc.NAME, r))
		} else {
			router.Handle("GET", r.FROM, ReverseProxy(mc.NAME, r))
		}
		fmt.Println("Module " + mc.NAME + " Hooked to Go-Proxy Server at - " + r.FROM + " => " + r.TO)
	}
	return nil
}

// BasicAuth - Authentification gin middleware
func BasicAuth(a *auth.BasicAuth) gin.HandlerFunc {
	realmHeader := "Basic realm=" + strconv.Quote(a.Realm)

	return func(c *gin.Context) {
		user := a.CheckAuth(c.Request)
		if user == "" {
			c.Header("WWW-Authenticate", realmHeader)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("user", user)
	}
}

//ReverseProxy - reverse proxy for mod
func ReverseProxy(modName string, r Route) gin.HandlerFunc {
	return func(c *gin.Context) {
		mod := GetManager().config.MODULES[modName]

		//CHECK IF MODULE IS ONLINE
		if mod.STATE == Online {
			//IF ROOT IS PRESENT REDIRECT TO IT
			if strings.Contains(mod.TYPES, "bind") && mod.BINDING.ROOT != "" {
				c.File(mod.BINDING.ROOT)

			} else if strings.Contains(mod.TYPES, "web") {
				//ELSE IF BINDING IS TYPE **WEB**
				//REVERSE PROXY TO IT
				url, err := url.Parse(mod.BINDING.PROTOCOL + "://" + mod.BINDING.ADDRESS + ":" + mod.BINDING.PORT + r.TO)
				if err != nil {
					log.Println(err)
				}
				proxy := NewSingleHostReverseProxy(url)
				proxy.ServeHTTP(c.Writer, c.Request)
			}
			//TODO HANDLE MORE STATES
		} else if mod.STATE == Loading || mod.STATE == Downloaded {
			//RETURN 503 WHILE MODULE IS LOADING
			c.HTML(503, "loading.html", nil)
		} else if mod.STATE == Stopped {
			c.String(504, "Module Stopped")
		} else if mod.STATE == Error {
			c.String(504, "Error")
		}
		//GetManager().config.MODULES[mc.NAME] = mod
	}
}

// NewSingleHostReverseProxy -
func NewSingleHostReverseProxy(target *url.URL) *httputil.ReverseProxy {
	targetQuery := target.RawQuery
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to default value
			req.Header.Set("User-Agent", "")
		}
	}
	return &httputil.ReverseProxy{Director: director}

}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b

}

/*Config - Global configuration */
type Config struct {
	NAME    string
	MODULES map[string]ModuleConfig
	VERSION int
	SERVER  ServerConfig
}

/*ModuleExecConfig - Module exec file informations */
type ModuleExecConfig struct {
	SRC  string
	MAIN string
	BIN  string
}

/*ServerConfig - Server configuration*/
type ServerConfig struct {
	ADDRESS  string
	PORT     string
	PATH     []Route
	PROTOCOL string
	ROOT     string
}

/*ModuleAuthConfig - Auth configuration*/
type ModuleAuthConfig struct {
	ENABLED bool
	TYPE    string
}

// Route - Route redirection
type Route struct {
	FROM string
	TO   string
}

//ModuleState - State of ModuleConfig
type ModuleState string

const (
	Unknown    ModuleState = "UNKNOWN"
	Loading    ModuleState = "LOADING"
	Online     ModuleState = "ONLINE"
	Stopped    ModuleState = "STOPPED"
	Downloaded ModuleState = "DOWNLOADED"
	Error      ModuleState = "ERROR"
)
