package main

import (
	"Mailer/utils"
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/boltdb/bolt"
)

type Item struct {
	NO           int
	Username     string
	Email        string
	Address      string
	Timestamp    string
	Activated    bool
	ActivateCode string
	IP           string
}

func (p *Item) toBytes() ([]byte, error) {
	var buf bytes.Buffer
	// Create an encoder and send a value.
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(p)
	if err != nil {
		log.Fatal("encode:", err)
		return nil, err
	}

	return buf.Bytes(), nil
}

func unmarshalItem(b []byte) (*Item, error) {
	u := &Item{}
	var buf = bytes.Buffer{}
	buf.Write(b)
	// Create a decoder and receive a value.
	dec := gob.NewDecoder(&buf)
	err := dec.Decode(u)
	if err != nil {
		log.Fatal("decode:", err)
		return nil, err
	}

	return u, nil
}

func saveEmail(email *Item) error {
	db, e := bolt.Open("email.db", 0600, nil)
	if e != nil {
		return e
	}
	defer db.Close()

	return db.Update(func(tx *bolt.Tx) error {
		b, e := tx.CreateBucketIfNotExists([]byte("EMAILS"))
		if e != nil {
			return e
		}

		v := time.Now().String()
		email.Timestamp = v
		bytes, err := email.toBytes()
		if err != nil {
			return err
		}
		return b.Put([]byte(email.Email), bytes)
	})
}

func getMail(key string) (*Item, error) {
	db, e := bolt.Open("email.db", 0600, nil)
	if e != nil {
		return nil, e
	}
	defer db.Close()

	var content []byte
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("EMAILS"))
		if b != nil {
			content = b.Get([]byte(key))
		}
		return nil
	})

	if content != nil {
		u, e := unmarshalItem(content)
		if e == nil {
			return u, nil
		}

		return nil, e
	}

	return nil, errors.New("key not exists")
}

func getEmails() ([]*Item, error) {
	db, e := bolt.Open("email.db", 0600, nil)
	if e != nil {
		return nil, e
	}
	defer db.Close()

	result := []*Item{}
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("EMAILS"))
		if b == nil {
			return errors.New("error b")
		}
		c := b.Cursor()
		if c == nil {
			return errors.New("nil cursor")
		}

		for k, v := c.First(); k != nil; k, v = c.Next() {
			item, ex := unmarshalItem(v)
			if ex != nil {
				continue
			}
			if item == nil {
				continue
			}
			result = append(result, item)
		}
		return nil
	})

	return result, nil
}

func static(w http.ResponseWriter, r *http.Request) {
	old := r.URL.Path
	r.URL.Path = strings.Replace(old, "/static", "/client", 1)
	//staticfs.ServeHTTP(w, r)
}

var timeouts map[string]time.Time = make(map[string]time.Time)

func returnResult(w http.ResponseWriter, result string) {
	content, err := ioutil.ReadFile("static/html/result.html")
	if err != nil {
		w.Write([]byte(err.Error()))
	}

	t, _ := template.New("webpage").Parse(string(content))
	err = t.Execute(w, result)
	if err != nil {
		w.Write([]byte(err.Error()))
	}
}

func activate(w http.ResponseWriter, req *http.Request) {
	value := req.URL.Query().Get("code")
	fmt.Println(value)

	r, err := base64.URLEncoding.DecodeString(value)
	if err != nil {
		returnResult(w, ("Invalid code format"))
	} else {
		strs := strings.Split(string(r), " ")
		if len(strs) == 2 {
			email := strs[0]
			code := strs[1]

			item, e := getMail(email)
			if e != nil {
				returnResult(w, "Email not register")
			} else {
				if item.ActivateCode == code {
					if item.Activated {
						returnResult(w, "Already activated before!")
						return
					}
					item.Activated = true
					ex := saveEmail(item)
					if ex != nil {
						returnResult(w, "Mail update failed")
					} else {
						returnResult(w, "Activate success!")
					}
				} else {
					returnResult(w, "Code not correct!")
				}
			}
		} else {
			returnResult(w, ("Invalid url"))
		}
	}
}

//send mail with defined
/*
func sendEmail(target string, value string) error {
	auth := smtp.PlainAuth(
		"",
		"support@ambr.org",
		"Tothemoon2050",
		"exmail.qq.com",
	)
	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	err := smtp.SendMail(
		"smtp.exmail.qq.com:465",
		auth,
		"support@ambr.org",
		[]string{target},
		[]byte(value),
	)

	return err
} */

func register(w http.ResponseWriter, req *http.Request) {

	email := req.PostFormValue("email")
	username := req.PostFormValue("username")
	address := req.PostFormValue("address")

	index := strings.Index(email, "@")
	if index >= 0 && index < len(email) {
		key := req.RemoteAddr
		keys := strings.Split(key, ":")
		key = keys[0]

		now := time.Now()
		value, ok := timeouts[key]
		if ok {
			diff := now.Sub(value)
			if diff.Seconds() <= 10 {
				returnResult(w, ("Error state"))
				return
			}
		}
		fmt.Println(key, now)
		timeouts[key] = now
		seed := rand.Int63()
		err := saveEmail(&Item{
			Username:     username,
			Email:        email,
			Address:      address,
			ActivateCode: string(seed),
			IP:           key,
		})
		if err != nil {
			returnResult(w, (err.Error()))
		}
		//send mail
		err = utils.SendMail(email, string(seed))
		if err != nil {
			returnResult(w, "Error to send email!")
			return
		}

		returnResult(w, "An activate mail has been sent to the mail you just inputed, please finish the activation ASAP!")
	} else {
		returnResult(w, "Error email!")
	}
}

func index(w http.ResponseWriter, req *http.Request) {
	content, err := ioutil.ReadFile("static/html/reg.html")
	if err != nil {
		w.Write([]byte(err.Error()))
	}

	t, _ := template.New("webpage").Parse(string(content))
	err = t.Execute(w, nil)
	if err != nil {
		w.Write([]byte(err.Error()))
	}
}

func emails(w http.ResponseWriter, req *http.Request) {
	content, err := ioutil.ReadFile("static/html/emails.html")
	if err != nil {
		w.Write([]byte(err.Error()))
	}

	t, _ := template.New("webpage").Parse(string(content))
	emails, e := getEmails()
	if e != nil {
		returnResult(w, e.Error())
		return
	}

	emails2 := []*Item{}
	for _, v := range emails {
		if v.Activated {
			emails2 = append(emails2, v)
		}
	}

	for i, v := range emails2 {
		v.NO = i + 1
	}

	data := struct {
		Items []*Item
	}{
		Items: emails2,
	}

	err = t.Execute(w, data)
	if err != nil {
		w.Write([]byte(err.Error()))
	}
}

func checkDetails(w http.ResponseWriter, req *http.Request) {
	content, err := ioutil.ReadFile("static/html/check.html")
	if err != nil {
		w.Write([]byte(err.Error()))
	} else {
		w.Write(content)
	}
}

func details(w http.ResponseWriter, req *http.Request) {
	email := req.PostFormValue("email")
	item, err := getMail(email)
	if err != nil {
		returnResult(w, "User not exists!")
	} else {
		content, err := ioutil.ReadFile("static/html/details.html")
		if err != nil {
			returnResult(w, err.Error())
		}

		t, e := template.New("webpage").Parse(string(content))
		if e != nil {
			returnResult(w, e.Error())
			return
		}

		err = t.Execute(w, item)
		if err != nil {
			w.Write([]byte(err.Error()))
		}
	}
}

func emailsall(w http.ResponseWriter, req *http.Request) {
	content, err := ioutil.ReadFile("static/html/emails.html")
	if err != nil {
		w.Write([]byte(err.Error()))
	}

	t, _ := template.New("webpage").Parse(string(content))
	emails, e := getEmails()
	if e != nil {
		returnResult(w, e.Error())
		return
	}

	for i, v := range emails {
		v.NO = i + 1
	}

	data := struct {
		Items []*Item
	}{
		Items: emails,
	}

	err = t.Execute(w, data)
	if err != nil {
		w.Write([]byte(err.Error()))
	}
}

func main2() {
	type Item2 struct {
		NO           int
		Username     string
		Email        string
		Address      string
		Timestamp    string
		Activated    bool
		ActivateCode string
	}
	db, e := bolt.Open("email2.db", 0600, nil)
	if e != nil {
		return
	}
	defer db.Close()

	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("EMAILS"))
		if b == nil {
			return errors.New("error b")
		}
		c := b.Cursor()
		if c == nil {
			return errors.New("nil cursor")
		}

		for k, v := c.First(); k != nil; k, v = c.Next() {

			u := &Item2{}
			var buf = bytes.Buffer{}
			buf.Write(v)
			// Create a decoder and receive a value.
			dec := gob.NewDecoder(&buf)
			err2 := dec.Decode(u)

			item, ex := u, err2
			if ex != nil {
				continue
			}
			if item == nil {
				continue
			}

			saveEmail(&Item{
				Username:     item.Username,
				Email:        item.Email,
				Address:      item.Address,
				Timestamp:    item.Timestamp,
				ActivateCode: item.ActivateCode,
				Activated:    item.Activated,
			})
		}
		return nil
	})
}

func main() {
	//http.FileServer(http.s)
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/register", register)
	http.HandleFunc("/emails", emails)
	http.HandleFunc("/check", checkDetails)
	http.HandleFunc("/details", details)
	http.HandleFunc("/emailsall", emailsall)
	http.HandleFunc("/activate/", activate)
	http.HandleFunc("/", index)
	http.ListenAndServe(":8001", nil)
}
