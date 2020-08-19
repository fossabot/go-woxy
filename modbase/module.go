package modbase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	gintemplate "github.com/foolin/gin-template"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"

	com "github.com/Wariie/go-woxy/com"
)

//HubAddress - Ip Address of thes hub
var HubAddress = "127.0.0.1"

//HubPort - Communication Port of the hub
var HubPort = "2000"

//ModuleAddress -
var ModuleAddress = "127.0.0.1"

//ModulePort -
var ModulePort = "2501"

//ResPath - Ressources main path (css, js, img, ......)
var ResPath = ""

type (
	/*HardwareUsage - Module hardware usage */
	HardwareUsage struct {
		CPU     byte
		MEM     byte
		NETWORK int
	}

	/*Module - Module*/
	Module interface {
		GetInfo() ModuleInfo
		GetInstanceName() string
		GetName() string
		Init()
		Register(string, func(*gin.Context), string)
		Run()
		Stop()
	}

	/*ModuleInfo - Module informations*/
	ModuleInfo struct {
		srv com.Server
		fmp string
	}

	/*ModuleImpl - Impl of Module*/
	ModuleImpl struct {
		Name         string
		InstanceName string
		Router       *gin.Engine
		Hash         string
		Secret       string
	}
)

//Stop - stop module
func (mod *ModuleImpl) Stop(c *gin.Context) {
	GetModManager().Shutdown(c)
}

//Run - start module function
//default ip 	-> 0.0.0.0
//default port	-> 2500
func (mod *ModuleImpl) Run() {
	log.Println("RUN - ", mod.GetName())
	//TODO ADD CONFIG FOR IP AND PORT
	if mod.connectToHub() {
		mod.serve(ModuleAddress, ModulePort)
	} else {
		mod.serve(ModuleAddress, ModulePort)
	}
}

//Init - init module
func (mod *ModuleImpl) Init() {
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	GetModManager().SetRouter(r)
	GetModManager().SetMod(mod)

	mod.readSecret()

	if ResPath == "" {
		ResPath = "ressources/"
	}
}

func (mod *ModuleImpl) readSecret() {
	b, err := ioutil.ReadFile(".secret")
	if err != nil {
		log.Println("Error reading server secret")
		os.Exit(2)
	}
	bs := sha256.Sum256(b)
	mod.Secret = string(bs[:])
}

//Register - register http handler for path
func (mod *ModuleImpl) Register(method string, path string, handler gin.HandlerFunc, typeM string) {
	log.Println("REGISTER - ", path)
	r := GetModManager().GetRouter()
	r.Handle(method, path, handler)

	if typeM == "WEB" {
		if len(path) > 1 {
			path += "/"
		}
		r.HTMLRender = gintemplate.Default()
		r.Use(static.ServeRoot(path+ResPath, "./"+ResPath))
	}
	GetModManager().SetRouter(r)
}

//GetName - get module name
func (mod *ModuleImpl) GetName() string {
	return mod.Name
}

//GetInstanceName - get module name
func (mod *ModuleImpl) GetInstanceName() string {
	return mod.InstanceName
}

/*serve -  */
func (mod *ModuleImpl) serve(ip string, port string) {
	r := GetModManager().GetRouter()
	r.POST("/cmd", cmd)

	Server := &http.Server{
		Addr:    ip + ":" + port,
		Handler: r,
	}

	GetModManager().SetServer(Server)
	GetModManager().SetRouter(r)
	GetModManager().SetMod(mod)

	if err := GetModManager().GetServer().ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}

}

func (mod *ModuleImpl) connectToHub() bool {
	log.Println("	HUB CONNECT")

	//CREATE CONNEXION REQUEST
	cr := com.ConnexionRequest{}
	cr.Generate(mod.GetName(), ModulePort, strconv.Itoa(os.Getpid()), mod.Secret)
	mod.Hash = cr.ModHash

	//SEND REQUEST
	body, err := com.SendRequest(com.Server{IP: HubAddress, Port: HubPort, Path: "", Protocol: "http"}, &cr, false)

	var crr com.ConnexionReponseRequest
	crr.Decode(bytes.NewBufferString(body).Bytes())

	s, err := strconv.ParseBool(crr.State)

	if s && err == nil {
		log.Println("		SUCCESS")
		//SET HASH
	} else {
		log.Println("		ERROR - ", err)
	}

	ModulePort = crr.Port
	return s && err == nil
}

type modManager struct {
	server *http.Server
	router *gin.Engine
	mod    *ModuleImpl
}

var singleton *modManager
var once sync.Once

//GetModManager -
func GetModManager() *modManager {
	once.Do(func() {
		singleton = &modManager{}
	})
	return singleton
}

func (sm *modManager) GetServer() *http.Server {
	return sm.server
}

func (sm *modManager) SetServer(s *http.Server) {
	sm.server = s
}

func (sm *modManager) GetRouter() *gin.Engine {
	return sm.router
}

func (sm *modManager) SetRouter(r *gin.Engine) {
	sm.router = r
}

func (sm *modManager) SetMod(m *ModuleImpl) {
	sm.mod = m
}

func (sm *modManager) GetMod() *ModuleImpl {
	return sm.mod
}

func (sm *modManager) GetSecret() string {
	return sm.mod.Secret
}

func (sm *modManager) Shutdown(c context.Context) {
	time.Sleep(10 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := sm.server.Shutdown(ctx); err != nil {
		log.Fatal("Server force to shutdown:", err)
	}
	log.Println("Server exiting")
}