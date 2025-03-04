package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var i int
var clients = make(map[*websocket.Conn]bool) // Connected clients
var broadcast = make(chan []byte, 1)         // Broadcast channel
var mutex = &sync.Mutex{}                    // Protect clients map

var chartClients = make(map[*websocket.Conn]bool)
var cartBroadcast = make(chan []byte)
var mutex1 = &sync.Mutex{}

func wsEndpoint(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
	}
	defer c.Close()

	mutex.Lock()
	clients[c] = true
	mutex.Unlock()

	for {
		i += 1
		_, message, err := c.ReadMessage()
		fmt.Println(c.RemoteAddr(), len(clients), len(broadcast))
		fmt.Println(message[:2], i)
		if err != nil {
			mutex.Lock()
			delete(clients, c)
			mutex.Unlock()
			break
		}
		// s := len(message)

		if s := len(message); s < 10 {
			continue
		}
		fmt.Println(message[:20])
		// break
		// broadcast <- message
		handleMess(message, c)
	}
}

func handleMess(msg []byte, cli *websocket.Conn) {
	// Grab the next message from the broadcast channel
	// select {
	// case message := <-broadcast:
	// mutex.Lock()

	for client := range clients {
		if cli != client {
			_ = client.WriteMessage(websocket.TextMessage, msg)
		}
		// if err != nil {
		// 	client.Close()
		// 	delete(clients, client)
		// }
	}
	// mutex.Unlock()
	// fmt.Println("start")
	// fmt.Println("end")
	// fmt.Println(len(broadcast))
	// Send the message to all connected clients
}

func wsChartEndpoint(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
	}
	defer c.Close()

	mutex1.Lock()
	chartClients[c] = true
	mutex1.Unlock()

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			fmt.Println("deleted")
			mutex.Lock()
			delete(clients, c)
			mutex.Unlock()
			break
		}
		fmt.Println(message)
		cartBroadcast <- message
		_ = <-cartBroadcast
	}
}

func handleChart() {
	for {
		// Grab the next message from the broadcast channel
		message := <-cartBroadcast
		fmt.Println(message)
		// Send the message to all connected clients
		mutex1.Lock()
		for client := range chartClients {
			err := client.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				client.Close()
				delete(clients, client)
			}
		}
		mutex1.Unlock()
	}
}

type Luckfox struct {
	ID        int
	Token     string
	SecretKey string
}

var luckfoxDB = map[string]Luckfox{
	"1": {ID: 1, Token: "152bdb30bdaf5d3c4447c0c0460d8060cb50beb04d8af84500c492cb2b373942", SecretKey: "QXJ0ZW0K"},
}

var imgChan = make(chan []byte, 10)

func postImageLuckfox(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	luckId, ok := mux.Vars(r)["id"]
	if !ok {
		fmt.Println("No id")
		return
	}
	if luckId == "" {
		fmt.Println("NO id")
		return
	}

	fmt.Println(luckId)
	if lf, ok := luckfoxDB[luckId]; ok {
		fmt.Println(luckId)
		authToken := r.Header.Get("Authorization")
		if authToken == lf.Token {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				return
			}
			// to save files
			// img, _, err := image.Decode(bytes.NewReader(data))
			// if err != nil {
			// 	log.Fatalln(err)
			// }
			// out, _ := os.Create("./img.jpeg")
			// defer out.Close()

			// err = jpeg.Encode(out, img, &jpeg.Options{Quality: 100})
			// if err != nil {
			// 	log.Println(err)
			//}

			l := uint32(len(data))
			buf := make([]byte, 4)
			k := uint32(1000)
			typer := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, l)
			binary.LittleEndian.PutUint32(typer, k)
			fin := append(buf, typer...)
			imgChan <- append(fin, data...)
		} else {
			fmt.Println("wrong token")
		}
	} else {
		fmt.Println("no this id")
	}
	return
}

func HandleUpload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
	fmt.Println("upload images")
	username, _, ok := r.BasicAuth()
	if !ok {
		w.Header().Add("WWW-Authenticate", `Basic realm="Give username and password"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "No basic auth present"}`))
		return
	}

	userRooms, ok := rooms[username]
	if !ok {
		w.Header().Add("WWW-Authenticate", `Basic realm="Give username and password"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "No basic auth present"}`))
		return
	}

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("Error parsing form")
		return
	}

	// get name of file
	name := r.Form.Get("username")
	if name == "" {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("specify a file name please")
		return
	}

	secretCode := r.Form.Get("password")
	if name == "" {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("specify a file name please")
		return
	}

	id := strconv.Itoa(len(userRooms))

	status, ok := secretExpired[secretCode]
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("specify a file name please")
		return
	}

	if status {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("specify a file name please")
		return
	}
	secretExpired[secretCode] = true
	tmpRoom := Room{ID: id, Employeer: name, Secretcode: secretCode}
	userRooms = append(userRooms, tmpRoom)
	// mux.Lock()
	rooms[username] = userRooms
	// mux.Unlock()
	// get file
	fhs := r.MultipartForm.File["images[]"]
	fmt.Println(len(fhs))
	for _, fh := range fhs {
		f, err := fh.Open()
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode("something went wrong")
			return
		}
		//
		data, err := io.ReadAll(f)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode("something went wrong")
			return
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			log.Fatalln(err)
		}

		out, _ := os.Create(fh.Filename)
		defer out.Close()

		// var opts jpeg.Options
		// opts.Quality = 1

		err = jpeg.Encode(out, img, &jpeg.Options{Quality: 100})
		//jpeg.Encode(out, img, nil)
		if err != nil {
			log.Println(err)
		}

		imgSize := uint32(len(data))
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, imgSize)

		imgChan <- append(buf, data...)
		defer f.Close()
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode("File uploaded successfully")

}

func runTCPClient() {
	var conn net.Conn
	servAddr := "localhost:8080"
	_, err := net.ResolveTCPAddr("tcp", servAddr)
	if err != nil {
		println("ResolveTCPAddr failed:", err.Error())
	}
loop1:
	for {
		timeout := time.Second
		conn, err = net.DialTimeout("tcp", servAddr, timeout)
		if err != nil {
			time.Sleep(timeout)
			continue
		}
		if conn != nil {
			defer conn.Close()
			break
		}
	}
	for {
		select {
		case img := <-imgChan:
			fmt.Println("new img")
			_, err := conn.Write([]byte(img))
			if err != nil {
				fmt.Println("Write to server failed:", err.Error())
				goto loop1
			}
		}
	}
}

var users = map[string]string{
	"test": "secret",
}

func DeleteUser(username string) {
	// дописать функцию чтобы каскадно
	// удалялсиь комнаты и осовобждался ключ

	if _, ok := users[username]; !ok {
		return
	}
	delete(users, username)

}

func isAuthorised(username, password string) bool {
	pass, ok := users[username]
	if !ok {
		return false
	}
	return password == pass
}

// секретный ключ а так же информация используется ли он
var secretExpired = map[string]bool{
	"QXJ0ZW0K": true,
}

type Room struct {
	ID         string `json:"id"`
	Employeer  string `json:"employeer"`
	Secretcode string `json:"secretcode"`
}

var rooms = map[string][]Room{
	"test": {
		Room{ID: "1", Employeer: "Egor Melnikov", Secretcode: "QXJ0ZW0K"},
	},
}

func getAvailableRooms(w http.ResponseWriter, r *http.Request) {
	fmt.Println("reach")
	w.Header().Add("Content-Type", "application/json")
	username, password, ok := r.BasicAuth()
	if !ok {
		w.Header().Add("WWW-Authenticate", `Basic realm="Give username and password"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "No basic auth present"}`))
		return
	}
	fmt.Println(username, password)

	if !isAuthorised(username, password) {
		w.Header().Add("WWW-Authenticate", `Basic realm="Give username and password"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Invalid username or password"}`))
		return
	}
	room, ok := rooms[username]
	if !ok {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode("")
		return
	}
	fmt.Println(len(room))
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(room)
	return
}

func registerUser(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(4 << 20)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// get name of file
	username := r.Form.Get("username")
	password := r.Form.Get("password")
	fmt.Println(username, password)
	if _, ok := users[username]; ok {
		w.WriteHeader(http.StatusConflict)
		// w.Write()
		return
	}
	users[username] = password
	w.WriteHeader(http.StatusOK)
}

func login(w http.ResponseWriter, r *http.Request) {
	fmt.Println("users")
	err := r.ParseMultipartForm(4 << 20)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// get name of file
	username := r.Form.Get("username")
	password := r.Form.Get("password")
	fmt.Println(username, password)
	// buf, _ := io.ReadAll(r.Body)
	// fmt.Println(buf)
	if isAuthorised(username, password) {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusConflict)
	return
}

func delRoom(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := mux.Vars(r)
	id := params["id"]
	_, ok := rooms[id]
	if !ok {
		log.Println("err")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// delete(room, id)
	// status, ok := secretExpired[room.Secretcode]
	w.WriteHeader(http.StatusOK)
	return
}

type Stats struct {
	Day       string
	Worktime  time.Duration
	Chilltime time.Duration
	Outtime   time.Duration
}

func (s Stats) MarshalJSON() ([]byte, error) {
	w := s.Worktime.Hours()
	c := s.Chilltime.Hours()
	o := s.Outtime.Hours()
	return json.Marshal(struct {
		Day       string  `json:"day"`
		Worktime  float64 `json:"worktime"`
		Chilltime float64 `json:"chilltime"`
		Outtime   float64 `json:"outtime"`
	}{
		Day:       s.Day,
		Worktime:  w,
		Chilltime: c,
		Outtime:   o,
	})
}

func (s *Stats) Add(dur time.Duration, mode string) {
	switch mode {
	case "worktime":
		s.Worktime += dur
		// fmt.Println(s.Worktime)
	case "chilltime":
		s.Chilltime += dur
		// fmt.Println(s.Chilltime)
	case "outtime":
		s.Outtime += dur
		// fmt.Println(s.Outtime)
	}

}

var statistics = map[string]map[string]*Stats{
	"43": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"44": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"45": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"46": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"47": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"48": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"49": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"50": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"52": {
		"a": &Stats{"Пн", time.Duration(4.8 * 3600 * 1000000000), time.Duration(2.2 * 3600 * 1000000000), time.Duration(1 * 3600 * 1000000000)},
		"b": &Stats{"Вт", time.Duration(6.9 * 3600 * 1000000000), time.Duration(1.1 * 3600 * 1000000000), time.Duration(0 * 3600 * 1000000000)},
		"c": &Stats{"Ср", time.Duration(7.8 * 3600 * 1000000000), time.Duration(0.2 * 3600 * 1000000000), time.Duration(0 * 3600 * 1000000000)},
		"d": &Stats{"Чт", time.Duration(7.2 * 3600 * 1000000000), time.Duration(0.8 * 3600 * 1000000000), time.Duration(0 * 3600 * 1000000000)},
		"e": &Stats{"Пт", time.Duration(5 * 3600 * 1000000000), time.Duration(1 * 3600 * 1000000000), time.Duration(2 * 3600 * 1000000000)},
	},
	"51": {
		"a": &Stats{"Пн", time.Duration(4.8 * 3600 * 1000000000), time.Duration(2.2 * 3600 * 1000000000), time.Duration(1 * 3600 * 1000000000)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"53": {
		"a": &Stats{"Пн", time.Duration(4.8 * 3600 * 1000000000), time.Duration(2.2 * 3600 * 1000000000), time.Duration(1 * 3600 * 1000000000)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"1": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"2": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"3": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"4": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"5": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"6": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"7": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
	"8": {
		"a": &Stats{"Пн", time.Duration(0), time.Duration(0), time.Duration(0)},
		"b": &Stats{"Вт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"c": &Stats{"Ср", time.Duration(0), time.Duration(0), time.Duration(0)},
		"d": &Stats{"Чт", time.Duration(0), time.Duration(0), time.Duration(0)},
		"e": &Stats{"Пт", time.Duration(0), time.Duration(0), time.Duration(0)},
	},
}

func getStatistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := mux.Vars(r)
	// id := params["id"]
	week := params["week"]
	data, ok := statistics[week]
	if !ok {
		log.Println("err")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	tmpdata := make([]*Stats, 0, len(data))
	for _, v := range data {
		fmt.Println(v)
		tmpdata = append(tmpdata, v)
	}
	json.NewEncoder(w).Encode(&tmpdata)
}

var wsChat = make(chan ServerResponse, 1)

type ServerResponse struct {
	Type string `json:"type"`
}

func getServerResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	var resp ServerResponse
	err := json.NewDecoder(r.Body).Decode(&resp)
	if err != nil {
		log.Println("err")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	wsChat <- resp
}

var status = "outtime"

type RespWS struct {
	Type string `json:"type"`
	Time string `json:"time"`
}

var lastTen = []RespWS{}

func webSocketClient() {
	var wor = 0
	var chi = 0
	var out = 0
	url := "ws://localhost:8081/ws/chart"
	day := &Stats{Day: "Сб", Worktime: time.Duration(0), Chilltime: time.Duration(0), Outtime: time.Duration(0)}
	statistics["52"]["f"] = day
loop:
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal(err)
	}

	defer c.Close()
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case resp := <-wsChat:
			status = resp.Type
			switch status {
			case "outtime":
				out += 1
			case "worktime":
				wor += 1
			case "chilltime":
				chi += 1
			}

		case <-ticker.C:
			status = "outtime"
			// d := time.Now().Sub(prev)
			// prev = time.Now()
			if val, ok := statistics["52"]; ok {
				if val1, ok := val["f"]; ok {
					val1.Add(time.Duration(5000000000), status)
					mv := max(out, wor, chi)
					if mv == 0 {
						status = "no video"
					} else {
						switch mv {
						case wor:
							status = "worktime"
						case chi:
							status = "chilltime"
						case out:
							status = "outtime"
						}
					}
					wor = 0
					chi = 0
					out = 0
					req := RespWS{Type: status, Time: time.Now().UTC().String()}
					lastTen = append(lastTen, req)
					if len(lastTen) > 10 {
						lastTen = lastTen[1:]
					}
					if err := c.WriteJSON(&req); err != nil {
						goto loop
					}
					fmt.Println(status)
				}

			}
		}

	}
}

func getLastTen(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(lastTen)
	if err != nil {
		log.Println("err")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	// wsChat <- resp
}

func authTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	username, password, ok := r.BasicAuth()
	if !ok {
		w.Header().Add("WWW-Authenticate", `Basic realm="Give username and password"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "No basic auth present"}`))
		return
	}
	fmt.Println(username, password)

	if !isAuthorised(username, password) {
		w.Header().Add("WWW-Authenticate", `Basic realm="Give username and password"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Invalid username or password"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	return
}

func main() {
	r := mux.NewRouter()
	var wg sync.WaitGroup
	wg.Add(1)
	r.HandleFunc("/authtest", authTest).Methods("GET")
	r.HandleFunc("/lastten", getLastTen).Methods("GET")
	r.HandleFunc("/sendresp", getServerResponse).Methods("POST")
	r.HandleFunc("/statistic/{id}/{week}", getStatistics).Methods("GET")
	r.HandleFunc("/delete/{id}", delRoom).Methods("PATCH")
	r.HandleFunc("/login", login).Methods("POST")
	r.HandleFunc("/room/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/test4.html")
	}).Methods("GET")
	r.HandleFunc("/newroom", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/test3.html")
	}).Methods("GET")
	r.HandleFunc("/rooms", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/test.html")
	}).Methods("GET")
	r.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/auth.html")
	}).Methods("GET")
	r.HandleFunc("/main", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/main.html")
	}).Methods("GET")
	r.HandleFunc("/register", registerUser).Methods("POST")
	r.HandleFunc("/rooms", getAvailableRooms).Methods("GET")
	r.HandleFunc("/image/{id}", postImageLuckfox).Methods("POST")
	r.HandleFunc("/ws/live", wsEndpoint)
	r.HandleFunc("/ws/chart", wsEndpoint)
	r.HandleFunc("/upload", HandleUpload).Methods("POST")
	defer func() { close(broadcast) }()
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("static/")))
	go runTCPClient()
	go webSocketClient()
	log.Fatal(http.ListenAndServe(":8081", r))
	wg.Wait()
}
