package main

import (

	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"html/template"
	_ "github.com/go-sql-driver/mysql"
	sql "database/sql"
	"github.com/siddontang/go/log"
	"strconv"
//	"regexp"
	"encoding/json"
//	"time"
//	"github.com/garyburd/redigo/redis"
	"github.com/Unknwon/goconfig"
//	"strconv"
	"bytes"
//	"os"
//	"bufio"
//	"io"
//	"strings"
	m "model"
	u "utils"
	es "gopkg.in/olivere/elastic.v3"
	"io/ioutil"
)

type FileData struct {
	ID int
	UID int
	Title string
}

var db *sql.DB
var err error
var username, password, url, address, redis_Pwd, mode, logLevel, redis_db string
var redis_Database int
var ConfError error
var esclient *es.Client
var cfg *goconfig.ConfigFile
var templateContent *template.Template
var Addr string
var Port string

//Mysql Redis ES init
func Init() {
	cfg, ConfError = goconfig.LoadConfigFile("config.ini")
	if ConfError != nil {
		log.Error("配置文件config.ini不存在,请将配置文件复制到运行目录下")
	}
	logLevel, ConfError = cfg.GetValue("Log", "logLevel")
	if ConfError != nil {
		log.SetLevel(log.LevelInfo)
	} else {
		log.SetLevelByName(logLevel)
	}
	username, ConfError = cfg.GetValue("MySQL", "username")
	if ConfError != nil {
		log.Error("读取数据库username错误")
	}
	password, ConfError = cfg.GetValue("MySQL", "password")
	if ConfError != nil {
		log.Error("读取数据库password错误")
	}
	url, ConfError = cfg.GetValue("MySQL", "url")
	if ConfError != nil {
		log.Error("读取数据库url错误")
	}

	var dataSourceName bytes.Buffer
	dataSourceName.WriteString(username)
	dataSourceName.WriteString(":")
	dataSourceName.WriteString(password)
	dataSourceName.WriteString("@")
	dataSourceName.WriteString(url)
	db, err = sql.Open("mysql", dataSourceName.String())
	if err != nil {
		log.Error(err.Error())
	}
	if err := db.Ping(); err != nil {
		panic("Error Connection database...")
	}
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(30)

	u.LISTMAX = 300
	u.PAGEMAX = 20
	u.NAVMAX = 5
	u.RANDMAX = 10
	u.InitCateMap()

	u.InitJieba()

	//init es
	esclient, err = es.NewClient()
	if err != nil {
		log.Error("failed to create es client")
	}
	m.TotalShares = m.GetTotalShares(esclient)
	m.TotalUsers = m.GetTotalUsers(esclient)

	m.MAX_USER, m.MIN_USER = m.GetUserMaxMINID(db)
	m.MAX_SHARE, m.MIN_SHARE = m.GetShareMaxMinID(db)
	m.MAX_KEYWORD, m.MIN_KEYWORD = m.GetKeywordMaxMinID(db)


	Addr, ConfError = cfg.GetValue("Server", "ip")
	if ConfError != nil {
		log.Error("读取数据库server错误")
	}


	Port, ConfError = cfg.GetValue("Server", "port")
	if ConfError != nil {
		log.Error("读取数据库port错误")
	}

	///
	u.InitRedis()

	//templateContent = string(ioutil.ReadFile("templates/index.html"))
	templ, err := ioutil.ReadFile("templates/index.html")
	if err == nil {
		templateContent = template.Must(template.New("tmp").Parse(string(templ)))
	} else {
		log.Error("failed to open template")
	}

}


func SetURL(url string, pv *m.PageVar) error {
	b, err := json.Marshal(pv)
	if err != nil {
		return err
	}
	str := string(b)
	err = u.SetRedis("bilisou" + url, str)
	log.Info("Set Cache for ", url)
	return err
}

func GetURL(url string) (*m.PageVar, error){
	str, err := u.GetRedis("bilisou" + url)
	if err != nil {
		return nil, err
	}

	pv := m.PageVar{}
	err = json.Unmarshal([]byte(str), &pv)
	log.Info("Get Cache for ", url)
	return &pv, err
}


func Index(w http.ResponseWriter, r *http.Request) {
	log.Info("ip = ", r.RemoteAddr, ", url = ", r.URL)
	pv, err := GetURL("home")
	if err == nil && pv != nil {
		render(w, pv)
	} else {
		pv := m.GenerateListPageVar(esclient, 0, 1)
		err = SetURL("home", pv)
		log.Info(err)
		render(w, pv)
	}

}

func ListShare(w http.ResponseWriter, r *http.Request) {

	log.Info("ip = ", r.RemoteAddr, ", url = ", r.URL)
	pv, err := GetURL(r.URL.Path)
	if err == nil && pv != nil {
		log.Info("it's from cache")
		render(w, pv)
		return
	}

	vars := mux.Vars(r)
	cat := vars["category"]
	cati, ok:= u.CAT_STR_INT[cat]
	if !ok {
		log.Info(err)
		cati = -1
	}

	p := vars["page"]
	if p == "" {
		p = "1"
	}
	pp, err:=strconv.Atoi(p)
	if err != nil {
		log.Info(err )
		return
	}
	pv = m.GenerateListPageVar(esclient, cati, pp)
	if pv != nil {
		render(w, pv)
	}
	SetURL(r.URL.Path, pv)
}

func ListUsers(w http.ResponseWriter, r *http.Request) {
	log.Info("ip = ", r.RemoteAddr, ", url = ", r.URL)
	log.Info("url = ", r.URL)
	pv, err := GetURL(r.URL.Path)
	if err == nil && pv != nil {
		log.Info("it's from cache")
		render(w, pv)
		return
	}

	vars := mux.Vars(r)
	p := vars["page"]
	if p == "" {
		p = "1"
	}
	pp, err:=strconv.Atoi(p)
	if err != nil {
		log.Info(err )
		return
	}
	pv = m.GenerateUlistPageVar(esclient, pp)
	if pv != nil {
		render(w, pv)
	}
	SetURL(r.URL.Path, pv)
}


func SearchShare(w http.ResponseWriter, r *http.Request) {
	log.Info("ip = ", r.RemoteAddr, ", url = ", r.URL)
	pv, err := GetURL(r.URL.Path)
	if err == nil && pv != nil {
		log.Info("it's from cache")
		render(w, pv)
		return
	}

	vars := mux.Vars(r)
	cat := vars["category"]
	cati, ok:= u.CAT_STR_INT[cat]
	if !ok {
		log.Info(err)
		cati = -1
	}

	keyword := vars["keyword"]
	if keyword == "" {
		log.Info(err)
	}

	p := vars["page"]
	if p == "" {
		p = "1"
	}

	pp, err:=strconv.Atoi(p)
	if err != nil {
		log.Info(err )
		return
	}
	m.KeywordHit(db,keyword)
	pv = m.GenerateSearchPageVar(esclient, cati, keyword, pp)
	if pv != nil {
		render(w, pv)
	}
	SetURL(r.URL.Path, pv)
}

func ShowShare(w http.ResponseWriter, r *http.Request) {
	log.Info("ip = ", r.RemoteAddr, ", url = ", r.URL)
	// break down the variables for easier assignment
	vars := mux.Vars(r)
	id := vars["dataid"]
	sp := m.GenerateSharePageVar(esclient, id)
	if sp != nil {
		render(w, sp)
	}
}

func ShowUser(w http.ResponseWriter, r *http.Request) {
	log.Info("ip = ", r.RemoteAddr, ", url = ", r.URL)
	vars := mux.Vars(r)
	uk := vars["uk"]
	p := vars["page"]
	if p == "" {
		p = "1"
	}

	pp, err:=strconv.Atoi(p)
	if err != nil {
		log.Info(err )
		pp = 1
	}

	pv := m.GenerateUserPageVar(esclient, uk, pp)
	if pv != nil {
		render(w, pv)
	}
}


func NotFound(w http.ResponseWriter, r *http.Request) {
	log.Info("ip = ", r.RemoteAddr, ", url = ", r.URL)
	pv := m.GenerateListPageVar(esclient, 0, 1)
	pv.Type = "lost"
	w.WriteHeader(http.StatusNotFound)
	if pv != nil {
		render(w, pv)
	}
	SetURL(r.URL.Path, pv)
}



func render(w http.ResponseWriter, data interface{}) {
	/*if err != nil {
		log.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}*/
	if err := templateContent.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}


func Robots(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User-agent: *\nDisallow:\n")
}

func main() {

	Init()

	//u.SetURL("aaa", "aabb")
	//log.Info(u.GetURL("aa"))

	mx := mux.NewRouter()

	mx.HandleFunc("/", Index)
	//list
	mx.HandleFunc("/list/{category}", ListShare)
	mx.HandleFunc("/list/{category}/", ListShare)
	mx.HandleFunc("/list/{category}/{page}", ListShare)
	mx.HandleFunc("/list/{category}/{page}/", ListShare)

	//ulist
	mx.HandleFunc("/ulist", ListUsers)
	mx.HandleFunc("/ulist/", ListUsers)
	mx.HandleFunc("/ulist/{page}", ListUsers)
	mx.HandleFunc("/ulist/{page}/", ListUsers)

	//search
	mx.HandleFunc("/search/{keyword}", SearchShare)
	mx.HandleFunc("/search/{category}/{keyword}", SearchShare)
	mx.HandleFunc("/search/{category}/{keyword}/", SearchShare)
	mx.HandleFunc("/search/{category}/{keyword}/{page}", SearchShare)
	mx.HandleFunc("/search/{category}/{keyword}/{page}/", SearchShare)

	//file
	mx.HandleFunc("/file/{dataid}", ShowShare)
	mx.HandleFunc("/file/{dataid}/", ShowShare)

	//user
	mx.HandleFunc("/user/{uk}", ShowUser)
	mx.HandleFunc("/user/{uk}/", ShowUser)
	mx.HandleFunc("/user/{uk}/{page}", ShowUser)
	mx.HandleFunc("/user/{uk}/{page}/", ShowUser)
	//server static
	mx.PathPrefix("/static").Handler(http.FileServer(http.Dir("./")))

	//for baidu
	mx.HandleFunc("/robots.txt", Robots)

	//not found
	mx.NotFoundHandler = http.HandlerFunc(NotFound)

	log.Info("Listening at ", Addr, ":", Port)
	http.ListenAndServe(Addr +":" + Port, mx)

}
