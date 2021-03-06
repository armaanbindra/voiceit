package app

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/garyburd/redigo/redis"
	"github.com/gilgameshskytrooper/voiceit/backend/structs"
	"github.com/gilgameshskytrooper/voiceit/backend/utils"
	"github.com/gilgameshskytrooper/voiceit/backend/video"
	"github.com/gorilla/mux"
)

func (app *App) Secret(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	if app.authenticateBrowserToken(username, r) {
		json.NewEncoder(w).Encode(structs.LoginSuccessStruct{Secret: "Epic Secret Content"})
	}
}

func (app *App) Login(w http.ResponseWriter, r *http.Request) {
	// Retreive the file and save to disk using the FormFile method
	file, header, err := r.FormFile("file")
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(403)
		return
	}
	username := header.Filename
	defer file.Close()

	// Check is username is saved in the database
	is_member, _ := redis.Bool(app.DB.Do("SISMEMBER", "users", username))

	if !is_member {
		w.WriteHeader(401)
		log.Println("Tried to login without a valid username")
		return
	}

	out, err1 := os.Create(utils.Pwd() + "files/" + username + ".mp4")

	if err1 != nil {
		w.WriteHeader(403)
		log.Println("Failed to os.Create")
		return
	}
	_, err2 := io.Copy(out, file)
	if err2 != nil {
		w.WriteHeader(403)
		log.Println("Failed to io.Copy")
		// os.Remove(utils.Pwd() + "files/" + username + ".mp4")
		return
	}

	out.Close()
	video.ConvertToH264MP4(utils.Pwd()+"files/", username)
	out, err = os.Open(utils.Pwd() + "files/" + username + ".mp4")
	if err != nil {
		log.Println("Failed to open converted .mp4 file")
		w.WriteHeader(403)
		// os.Remove(utils.Pwd() + "files/" + username + ".mp4")
		return
	}

	response := structs.VideoVerificationResponse{}
	userid, _ := redis.String(app.DB.Do("HGET", "logins", username+":userid"))
	json.Unmarshal(app.VoiceIt.VideoVerification(userid, "en-US", utils.Pwd()+"files/"+username+".mp4").Bytes(), &response)
	if response.ResponseCode != "SUCC" && !app.ForceSucceedLogin { // Verification failed. Return user to root
		w.WriteHeader(403)
		log.Println("Failed to log in")
		log.Println("mesage:", response.Message)
		log.Println("ResponseCode:", response.ResponseCode)
		app.ForceSucceedLogin = true
		// os.Remove(utils.Pwd() + "files/" + username + ".mp4")
		return
	}

	log.Println("app.ForceSucceedLogin set to true (to show what an authenticated user would see)")
	app.ForceSucceedLogin = false
	out.Close()
	app.setSession(username, w)
	json.NewEncoder(w).Encode(structs.LoginSuccessStruct{Secret: "Ever notice Jennifer from Back to the Future changed actresses between I and II? Claudia Wells, the first Jennifer, was unable to reprise the role due to her mother becoming ill. The studio recast Elisabeth Shue for Back to the Future II and III and reshot the final footage of BTTF with Shue instead of Wells for BTTF 2’s opening."})
}

func (app *App) Register(w http.ResponseWriter, r *http.Request) {
	// Grab file, save it to disk
	file, header, err := r.FormFile("file")
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(403)
		return
	}
	username := header.Filename

	defer file.Close()

	out, err1 := os.Create(utils.Pwd() + "files/" + username + ".mp4")

	if err1 != nil {
		w.WriteHeader(403)
		log.Println("Failed to os.Create")
		return
	}

	_, err2 := io.Copy(out, file)
	if err2 != nil {
		out.Close()
		// os.Remove(utils.Pwd() + "files/" + username + ".mp4")
		w.WriteHeader(403)
		log.Println("Failed to io.Copy")
		return
	}
	out.Close()
	video.ConvertToH264MP4(utils.Pwd()+"files/", username)
	out, err = os.Open(utils.Pwd() + "files/" + username + ".mp4")
	if err != nil {
		log.Println("Failed to open converted .mp4 file")
		w.WriteHeader(403)
		return
	}

	// Check if user already exists in database, return user to root if user already exists in the database
	is_member, _ := redis.Bool(app.DB.Do("SISMEMBER", "users", username))
	if is_member {
		out.Close()
		// os.Remove(utils.Pwd() + "files/" + username + ".mp4")
		log.Println("User tried to register existing username")
		w.WriteHeader(403)
		return
	}

	// Since doesn't already exist in the system, add user to database
	app.DB.Do("SADD", "users", username)

	// Register user in VoiceIt API
	create_user_response := structs.CreateNewUserResponse{}
	json.Unmarshal(app.VoiceIt.CreateUser().Bytes(), &create_user_response)

	if create_user_response.ResponseCode != "SUCC" {
		out.Close()
		// os.Remove(utils.Pwd() + "files/" + username + ".mp4")
		log.Println("Create user caused failure\n" + create_user_response.Message)
		w.WriteHeader(403)
		return
	}

	app.DB.Do("HSET", "logins", username+":userid", create_user_response.UserID)

	// Create new video enrollment for user for given group
	create_user_video_enrollment_response := structs.CreateUserVideoEnrollmentResponse{}

	json.Unmarshal(
		app.VoiceIt.CreateVideoEnrollment(
			create_user_response.UserID,
			"en-US",
			utils.Pwd()+"files/"+username+".mp4").Bytes(),
		&create_user_video_enrollment_response)

	if create_user_video_enrollment_response.ResponseCode != "SUCC" {
		out.Close()
		// os.Remove(utils.Pwd() + "files/" + username + ".mp4")
		log.Println(create_user_video_enrollment_response.Message)
		log.Println("Creating user video enrollment failed.")
		w.WriteHeader(403)
		return
	}

	out.Close()
	// os.Remove(utils.Pwd() + "files/" + username + ".mp4")
	w.WriteHeader(302)
}
