package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

type User struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Profile struct {
		RealName    string `json:"real_name"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		ImageURL    string `json:"image_192"`
	} `json:"profile"`
	Deleted bool `json:"deleted"`
	IsBot   bool `json:"is_bot"`
}

type Channel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Topic struct {
		Value string `json:"value"`
	} `json:"topic"`
	Purpose struct {
		Value string `json:"value"`
	} `json:"purpose"`
}

type Message struct {
	ChannelID       string
	UserID          string `json:"user"`
	BotID           string `json:"bot_id"`
	BotUsername     string `json:"username"`
	Timestamp       string `json:"ts"`
	Text            string `json:"text"`
	ThreadTimestamp string `json:"thread_ts"`
	ParentUserID    string `json:"parent_user_id"`
}

const (
	ZIPFILE_PATH      = "/slack-export.zip"
	EXTRACTION_DIR    = "/extracted"
	USERS_FILEPATH    = EXTRACTION_DIR + "/users.json"
	CHANNELS_FILEPATH = EXTRACTION_DIR + "/channels.json"
	SLACKBOT_ID       = "USLACKBOT"
)

var (
	db         *sql.DB
	userSet    = make(map[string]string)
	channelSet = make(map[string]string)
	messageSet = make(map[string]bool)
)

func CheckError(err error) {
	if err != nil {
		panic(err)
	}
}

func unzipFile(file *zip.File, dest string) error {
	// Check if file paths are not vulnerable to Zip Slip
	filePath := filepath.Join(dest, file.Name)
	if !strings.HasPrefix(filePath, filepath.Clean(dest)+string(os.PathSeparator)) {
		return fmt.Errorf("%s: illegal file path", filePath)
	}

	if file.FileInfo().IsDir() {
		if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
			return err
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		return err
	}

	destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer destFile.Close()

	zipFile, err := file.Open()
	if err != nil {
		return err
	}
	defer zipFile.Close()

	if _, err := io.Copy(destFile, zipFile); err != nil {
		return err
	}

	return nil
}

func unzipSource(src, dest string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()

	dest, err = filepath.Abs(dest)
	if err != nil {
		return err
	}

	for _, file := range reader.File {
		err := unzipFile(file, dest)
		if err != nil {
			return err
		}
	}

	return nil
}

func queryExistingContent() {
	rows, err := db.Query("SELECT id, name FROM users;")
	CheckError(err)
	defer rows.Close()

	for rows.Next() {
		var user User
		err = rows.Scan(&user.ID, &user.Name)
		CheckError(err)
		userSet[user.ID] = user.Name
	}

	rows, err = db.Query("SELECT id, name FROM channels;")
	CheckError(err)
	defer rows.Close()

	for rows.Next() {
		var channel Channel
		err = rows.Scan(&channel.ID, &channel.Name)
		CheckError(err)
		channelSet[channel.ID] = channel.Name
	}

	rows, err = db.Query("SELECT channel_id, user_id, EXTRACT(epoch FROM ts) AS ts FROM messages;")
	CheckError(err)
	defer rows.Close()

	for rows.Next() {
		var message Message
		err = rows.Scan(&message.ChannelID, &message.UserID, &message.Timestamp)
		CheckError(err)
		messageSet[message.ChannelID+message.UserID+message.Timestamp] = true
	}

	log.Println("Digester found " + fmt.Sprint(len(userSet)) + " users, " + fmt.Sprint(len(channelSet)) + " channels and " + fmt.Sprint(len(messageSet)) + " messages already in the tummy.")
}

func main() {
	host := os.Getenv("TUMMY_HOST")
	port, err := strconv.Atoi(os.Getenv("TUMMY_PORT"))
	CheckError(err)
	user := os.Getenv("TUMMY_USERNAME")
	password := os.Getenv("TUMMY_PASSWORD")
	dbname := os.Getenv("TUMMY_DB")

	psqlconn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)

	db, err = sql.Open("postgres", psqlconn)
	CheckError(err)
	defer db.Close()

	err = db.Ping()
	CheckError(err)
	log.Println("Digester is now successfully connected to the tummy!")

	row := db.QueryRow("SELECT COUNT(*) FROM messages;")
	existingMessagesCount := 0
	err = row.Scan(&existingMessagesCount)
	CheckError(err)

	if existingMessagesCount > 0 {
		log.Printf("Digester %d found messages in tummy. Querying existing content...", existingMessagesCount)
		queryExistingContent()
	} else {
		log.Println("Digester found an empty tummy. Starting to digest the Slack export...")
	}

	err = unzipSource(ZIPFILE_PATH, EXTRACTION_DIR)
	CheckError(err)
	log.Println("Slack export has been successfully extracted!")

	usersFile, err := os.ReadFile(USERS_FILEPATH)
	CheckError(err)

	var users []User
	err = json.Unmarshal(usersFile, &users)
	CheckError(err)

	_, slackBotExists := userSet[SLACKBOT_ID]
	if !slackBotExists {
		// Slackbot is a special bot that is not included in the users.json file and also doesnt have a bot_id
		slackbot := User{
			ID:      SLACKBOT_ID,
			Name:    "slackbot",
			Deleted: false,
			IsBot:   true,
		}
		slackbot.Profile.DisplayName = "Slackbot"
		slackbot.Profile.RealName = "Slackbot"
		users = append(users, slackbot)
		log.Println("Slackbot not found in tummy. Adding it to the users list.")
	}

	newUsersCount := 0
	oldUsersUpdatedCount := 0
	for _, user := range users {
		_, userExists := userSet[user.ID]
		if userExists {
			query := "UPDATE users SET name = $1, real_name = $2, display_name = $3, email = $4, deleted = $5, is_bot = $6, image_url = $7 WHERE id = $8;"
			_, err = db.Exec(query, user.Name, user.Profile.RealName, user.Profile.DisplayName, user.Profile.Email, user.Deleted, user.IsBot, user.Profile.ImageURL, user.ID)
			CheckError(err)
			oldUsersUpdatedCount++
			continue
		}
		query := "INSERT INTO users (id, name, real_name, display_name, email, deleted, is_bot, image_url) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);"
		_, err = db.Exec(query, user.ID, user.Name, user.Profile.RealName, user.Profile.DisplayName, user.Profile.Email, user.Deleted, user.IsBot, user.Profile.ImageURL)
		CheckError(err)
		newUsersCount++
		userSet[user.ID] = user.Name
	}
	if newUsersCount > 0 {
		log.Println("Digester digested " + fmt.Sprint(newUsersCount) + " new users and sent to the tummy.")
	} else {
		log.Println("Digester found no new users.")
	}
	if oldUsersUpdatedCount > 0 {
		log.Println("Digester re-digested " + fmt.Sprint(oldUsersUpdatedCount) + " existing users in the tummy.")
	} else {
		log.Println("Digester found no need to re-digest any existing user.")
	}

	channelsFile, err := os.ReadFile(CHANNELS_FILEPATH)
	CheckError(err)

	var channels []Channel
	err = json.Unmarshal(channelsFile, &channels)
	CheckError(err)

	newChannelsCount := 0
	existingChannelsUpdatedCount := 0
	for _, channel := range channels {
		_, channelExists := channelSet[channel.ID]
		if channelExists {
			query := "UPDATE channels SET name = $1, topic = $2, purpose = $3 WHERE id = $4;"
			_, err = db.Exec(query, channel.Name, channel.Topic.Value, channel.Purpose.Value, channel.ID)
			CheckError(err)
			existingChannelsUpdatedCount++
			continue
		}
		query := "INSERT INTO channels (id, name, topic, purpose) VALUES ($1, $2, $3, $4)"
		_, err = db.Exec(query, channel.ID, channel.Name, channel.Topic.Value, channel.Purpose.Value)
		CheckError(err)
		newChannelsCount++
		channelSet[channel.ID] = channel.Name
	}
	if newChannelsCount > 0 {
		log.Println("Digester digested " + fmt.Sprint(newChannelsCount) + " new channels and sent to the tummy.")
	} else {
		log.Println("Digester found no new channels.")
	}
	if existingChannelsUpdatedCount > 0 {
		log.Println("Digester re-digested " + fmt.Sprint(existingChannelsUpdatedCount) + " existing channels in the tummy.")
	} else {
		log.Println("Digester found no need to re-digest any existing channel.")
	}

	newBotsCount := 0
	newMessagesCount := 0
	for _, channel := range channels {
		messagesDirPath := filepath.Join(EXTRACTION_DIR, channel.Name)
		messageFiles, err := os.ReadDir(messagesDirPath)
		CheckError(err)

		for _, messageFile := range messageFiles {
			if messageFile.Name() == "canvas_in_the_conversation.json" {
				continue
			}

			messageFilePath := filepath.Join(messagesDirPath, messageFile.Name())
			messagesFile, err := os.ReadFile(messageFilePath)
			CheckError(err)

			var messages []Message
			err = json.Unmarshal(messagesFile, &messages)
			CheckError(err)

			for _, message := range messages {
				message.ChannelID = channel.ID
				if message.UserID == "" {
					if message.BotID == "" {
						continue
					} else {
						if _, userExists := userSet[message.BotID]; userExists {
							message.UserID = message.BotID
						} else {
							newBot := User{
								ID:      message.BotID,
								Name:    message.BotUsername,
								Deleted: false,
								IsBot:   true,
							}
							newBot.Profile.DisplayName = message.BotUsername
							newBot.Profile.RealName = message.BotUsername
							users = append(users, newBot)

							query := "INSERT INTO users (id, name, real_name, display_name, email, deleted, is_bot, image_url) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);"
							_, err = db.Exec(query, newBot.ID, newBot.Name, newBot.Profile.RealName, newBot.Profile.DisplayName, newBot.Profile.Email, newBot.Deleted, newBot.IsBot, newBot.Profile.ImageURL)
							CheckError(err)
							newBotsCount++
							userSet[newBot.ID] = newBot.Name

							message.UserID = message.BotID
						}
					}
				}

				if messageSet[message.ChannelID+message.UserID+message.Timestamp] {
					continue
				}

				if message.ThreadTimestamp != "" {
					query := "INSERT INTO messages (channel_id, user_id, ts, msg_text, parent_user_id, thread_ts) VALUES ($1, $2, TIMESTAMP 'epoch' + $3 * INTERVAL '1 second', $4, $5, TIMESTAMP 'epoch' + $6 * INTERVAL '1 second');"
					_, err = db.Exec(query, message.ChannelID, message.UserID, message.Timestamp, message.Text, message.ParentUserID, message.ThreadTimestamp)
				} else {
					query := "INSERT INTO messages (channel_id, user_id, ts, msg_text, parent_user_id) VALUES ($1, $2, TIMESTAMP 'epoch' + $3 * INTERVAL '1 second', $4, $5);"
					_, err = db.Exec(query, message.ChannelID, message.UserID, message.Timestamp, message.Text, message.ParentUserID)
				}
				CheckError(err)
				newMessagesCount++
			}
		}
	}
	log.Println("Digester digested " + fmt.Sprint(newBotsCount) + " new bots and " + fmt.Sprint(newMessagesCount) + " new messages and sent to the tummy.")
}
