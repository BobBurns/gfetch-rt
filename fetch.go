package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"

	"gopkg.in/natefinch/lumberjack.v2"
)

// can use this in the future to
// build more granular query
type message struct {
	time     int64
	gmail_ID string
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func main() {
	// credential file obtained from https://developers.google.com/docs/api/quickstart/go

	b, err := ioutil.ReadFile("techpluscred.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, gmail.GmailSendScope, gmail.GmailReadonlyScope, gmail.GmailModifyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := gmail.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Gmail client: %v", err)
	}

	// set up logging
	log.SetOutput(&lumberjack.Logger{
		Filename: "gfetch.log",
		MaxSize:  5,
		MaxAge:   7,
		Compress: true,
	})

	// loop to check for unread messages. If yes,
	// hand off to rt-mailgate and delete unread label
	for {

		req := srv.Users.Messages.List("me").LabelIds("UNREAD").MaxResults(1)

		r, err := req.Do()

		if err != nil {
			log.Fatalf("unable to retrieve messages: %s", err)
		}

		numMess := len(r.Messages)
		fmt.Println("nummess: ", numMess)
		if numMess > 0 {
			for i := 0; i < numMess; i++ {

				fmt.Println("ID: ", r.Messages[i].Id)
				reqMessage := srv.Users.Messages.Get("me", r.Messages[i].Id).Format("RAW")
				r2, err := reqMessage.Do()
				if err != nil {
					log.Fatal(err)
				}
				log.Println("snip: ", r2.Snippet)
				// here is where I hand off to rt-gate
				/*
					rt-testsupport:         "|/opt/rt4/bin/rt-mailgate --queue testsupport --action correspond --url http://testsupport.sccoe.santacruz.k12.ca.us"
				*/
				cmd := exec.Command("/opt/rt4/bin/rt-mailgate", "--queue", "techplus", "--action", "correspond", "--url", "http://testsupport.sccoe.santacruz.k12.ca.us", "--debug")

				decMess, err := base64.URLEncoding.DecodeString(r2.Raw)
				if err != nil {
					log.Fatal(err)
				}

				stdin, err := cmd.StdinPipe()
				if err != nil {
					log.Fatal(err)
				}
				go func() {
					defer stdin.Close()
					io.WriteString(stdin, string(decMess))
				}()

				out, err := cmd.CombinedOutput()
				if err != nil {
					log.Fatal(err)
				}

				log.Printf("%s\n", out)

				/* func (r *UsersMessagesService) Modify(userId string,
				 * id string, modifymessagerequest
				 * *ModifyMessageRequest) *UsersMessagesModifyCall
				 */
				mc := &gmail.ModifyMessageRequest{
					RemoveLabelIds: []string{"UNREAD"},
				}

				modReq := srv.Users.Messages.Modify("me", r.Messages[i].Id, mc)
				_, err = modReq.Do()
				if err != nil {
					log.Fatal(err)
				}

			}
		}
		time.Sleep(time.Second * 20)

	}
}
