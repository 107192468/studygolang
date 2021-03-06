// Copyright 2013 The StudyGolang Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// http://studygolang.com
// Author：polaris	studygolang@gmail.com

package controller

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"config"
	"filter"
	"github.com/dchest/captcha"
	"github.com/gorilla/sessions"
	"github.com/studygolang/mux"
	"logger"
	"service"
	"util"
)

// 用户注册
// uri: /account/register{json:(|.json)}
func RegisterHandler(rw http.ResponseWriter, req *http.Request) {
	if _, ok := filter.CurrentUser(req); ok {
		util.Redirect(rw, req, "/")
		return
	}

	vars := mux.Vars(req)
	username := req.PostFormValue("username")
	// 请求注册页面
	if username == "" || req.Method != "POST" || vars["json"] == "" {
		filter.SetData(req, map[string]interface{}{"captchaId": captcha.NewLen(4)})
		req.Form.Set(filter.CONTENT_TPL_KEY, "/template/register.html")
		return
	}

	// 校验验证码
	if !captcha.VerifyString(req.PostFormValue("captchaid"), req.PostFormValue("captchaSolution")) {
		fmt.Fprint(rw, `{"ok": 0, "error":"验证码错误"}`)
		return
	}

	// 入库
	errMsg, err := service.CreateUser(req.PostForm)
	if err != nil {
		// bugfix：http://studygolang.com/topics/255
		if errMsg == "" {
			errMsg = err.Error()
		}
		fmt.Fprint(rw, `{"ok": 0, "error":"`, errMsg, `"}`)
		return
	}

	// 注册成功，自动为其登录
	setCookie(rw, req, req.PostFormValue("username"))
	// 发送欢迎邮件
	go sendWelcomeMail([]string{req.PostFormValue("email")})
	fmt.Fprint(rw, `{"ok": 1, "msg":"注册成功"}`)
}

func sendWelcomeMail(email []string) {
	content := `Welcome to Study Golang.<br><br>
欢迎您，成功注册成为 Go语言中文网 | Go语言学习园地 会员<br><br>
Golang中文社区是一个Go语言技术社区，完全用Go语言开发。我们为gopher们提供一个好的学习交流场所。加入到社区中来，参与分享，学习，不断提高吧。前往 <a href="http://studygolang.com">Golang中文社区 | Go语言学习园地</a><br>
<div style="text-align:right;">&copy;2012-2015 studygolang.com Go语言中文网 | Golang中文社区 | Go语言学习园地</div>`
	service.SendMail("Golang中文社区 | Go语言学习园地 注册成功通知", content, email)
}

// 登录
// uri : /account/login{json:(|.json)}
func LoginHandler(rw http.ResponseWriter, req *http.Request) {
	username := req.PostFormValue("username")
	if username == "" || req.Method != "POST" {
		filter.SetData(req, map[string]interface{}{"error": "非法请求"})
		req.Form.Set(filter.CONTENT_TPL_KEY, "/template/login.html")
		return
	}

	vars := mux.Vars(req)

	suffix := vars["json"]

	// 处理用户登录
	passwd := req.PostFormValue("passwd")
	userLogin, err := service.Login(username, passwd)
	if err != nil {
		if suffix != "" {
			logger.Errorln("login error:", err)
			fmt.Fprint(rw, `{"ok":0,"error":"`+err.Error()+`"}`)
			return
		}

		req.Form.Set(filter.CONTENT_TPL_KEY, "/template/login.html")
		filter.SetData(req, map[string]interface{}{"username": username, "error": err.Error()})
		return
	}
	logger.Debugf("remember_me is %q\n", req.FormValue("remember_me"))
	// 登录成功，种cookie
	setCookie(rw, req, userLogin.Username)

	if suffix != "" {
		fmt.Fprint(rw, `{"ok":1,"msg":"success"}`)
		return
	}

	// 支持跳转到源页面
	uri := "/"
	values := filter.NewFlash(rw, req).Flashes("uri")
	if values != nil {
		uri = values[0].(string)
	}
	logger.Debugln("uri===", uri)
	util.Redirect(rw, req, uri)
}

// 用户编辑个人信息
func AccountEditHandler(rw http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	curUser, _ := filter.CurrentUser(req)
	if req.Method != "POST" || vars["json"] == "" {
		// 获取用户信息
		user := service.FindUserByUsername(curUser["username"].(string))
		// 设置模板数据
		filter.SetData(req, map[string]interface{}{"user": user, "default_avatars": service.DefaultAvatars})
		req.Form.Set(filter.CONTENT_TPL_KEY, "/template/user/edit.html")
		return
	}

	req.PostForm.Set("username", curUser["username"].(string))

	if req.PostFormValue("open") != "1" {
		req.PostForm.Set("open", "0")
	}

	// 更新个人信息
	errMsg, err := service.UpdateUser(req.PostForm)
	if err != nil {
		fmt.Fprint(rw, `{"ok": 0, "error":"`, errMsg, `"}`)
		return
	}
	fmt.Fprint(rw, `{"ok": 1, "msg":"个人资料更新成功!"}`)
}

// 更换头像
// uri: /account/change_avatar.json
func ChangeAvatarHandler(rw http.ResponseWriter, req *http.Request) {
	curUser, _ := filter.CurrentUser(req)
	avatar, ok := req.PostForm["avatar"]
	if !ok {
		fmt.Fprint(rw, `{"ok": 0, "error":"非法请求！"}`)
		return
	}
	err := service.ChangeAvatar(curUser["uid"].(int), avatar[0])
	if err != nil {
		fmt.Fprint(rw, `{"ok": 0, "error":"更换头像失败"}`)
		return
	}

	fmt.Fprint(rw, `{"ok": 1, "msg":"更换头像成功!"}`)
}

// 修改密码
// uri: /account/changepwd.json
func ChangePwdHandler(rw http.ResponseWriter, req *http.Request) {
	curUser, _ := filter.CurrentUser(req)
	username := curUser["username"].(string)

	curPasswd := req.PostFormValue("cur_passwd")
	_, err := service.Login(username, curPasswd)
	if err != nil {
		// 原密码错误
		fmt.Fprint(rw, `{"ok": 0, "error": "原密码填写错误!"}`)
		return
	}
	// 更新密码
	errMsg, err := service.UpdatePasswd(username, req.PostFormValue("passwd"))
	if err != nil {
		fmt.Fprint(rw, `{"ok": 0, "error":"`, errMsg, `"}`)
		return
	}
	fmt.Fprint(rw, `{"ok": 1, "msg":"密码修改成功!"}`)
}

// 保存uuid和email的对应关系（TODO:重启如何处理，有效期问题）
var resetPwdMap = map[string]string{}

// 忘记密码
// uri: /account/forgetpwd
func ForgetPasswdHandler(rw http.ResponseWriter, req *http.Request) {
	if _, ok := filter.CurrentUser(req); ok {
		util.Redirect(rw, req, "/")
		return
	}
	req.Form.Set(filter.CONTENT_TPL_KEY, "/template/user/forget_pwd.html")
	data := map[string]interface{}{"activeUsers": "active"}
	email := req.FormValue("email")
	if email == "" || req.Method != "POST" {
		filter.SetData(req, data)
		return
	}
	// 校验email是否存在
	if service.EmailExists(email) {
		var uuid string
		for {
			uuid = util.GenUUID()
			if _, ok := resetPwdMap[uuid]; !ok {
				resetPwdMap[uuid] = email
				break
			}
			logger.Infoln("GenUUID 冲突....")
		}
		var emailUrl string
		if strings.HasSuffix(email, "@gmail.com") {
			emailUrl = "http://mail.google.com"
		} else {
			pos := strings.LastIndex(email, "@")
			emailUrl = "http://mail." + email[pos+1:]
		}
		data["success"] = template.HTML(`一封包含了重设密码链接的邮件已经发送到您的注册邮箱，按照邮件中的提示，即可重设您的密码。<a href="` + emailUrl + `" target="_blank">立即前往邮箱</a>`)
		go sendResetpwdMail(email, uuid)
	} else {
		data["error"] = "该邮箱没有在本社区注册过！"
	}
	filter.SetData(req, data)
}

// 重置密码
// uri: /account/resetpwd
func ResetPasswdHandler(rw http.ResponseWriter, req *http.Request) {
	if _, ok := filter.CurrentUser(req); ok {
		util.Redirect(rw, req, "/")
		return
	}
	uuid := req.FormValue("code")
	if uuid == "" {
		util.Redirect(rw, req, "/account/login")
		return
	}
	req.Form.Set(filter.CONTENT_TPL_KEY, "/template/user/reset_pwd.html")
	data := map[string]interface{}{"activeUsers": "active"}

	passwd := req.FormValue("passwd")
	email, ok := resetPwdMap[uuid]
	if !ok {
		// 是提交重置密码
		if passwd != "" && req.Method == "POST" {
			data["error"] = template.HTML(`非法请求！<p>将在<span id="jumpTo">3</span>秒后跳转到<a href="/" id="jump_url">首页</a></p>`)
		} else {
			data["error"] = template.HTML(`链接无效或过期，请重新操作。<a href="/account/forgetpwd">忘记密码？</a>`)
		}
		filter.SetData(req, data)
		return
	}

	data["valid"] = true
	data["code"] = uuid
	// 提交修改密码
	if passwd != "" && req.Method == "POST" {
		// 简单校验
		if len(passwd) < 6 || len(passwd) > 32 {
			data["error"] = "密码长度必须在6到32个字符之间"
		} else if passwd != req.FormValue("pass2") {
			data["error"] = "两次密码输入不一致"
		} else {
			// 更新密码
			_, err := service.UpdatePasswd(email, passwd)
			if err != nil {
				data["error"] = "对不起，服务器错误，请重试！"
			} else {
				data["success"] = template.HTML(`密码重置成功，<p>将在<span id="jumpTo">3</span>秒后跳转到<a href="/account/login" id="jump_url">登录</a>页面</p>`)
			}
		}
	}
	filter.SetData(req, data)
}

// 发重置密码邮件
func sendResetpwdMail(email, uuid string) {
	content := `您好，` + email + `,<br/><br/>
&nbsp;&nbsp;&nbsp;&nbsp;我们的系统收到一个请求，说您希望通过电子邮件重新设置您在 <a href="http://` + config.Config["domain"] + `">Golang中文社区</a> 的密码。您可以点击下面的链接重设密码：<br/><br/>

&nbsp;&nbsp;&nbsp;&nbsp;http://` + config.Config["domain"] + `/account/resetpwd?code=` + uuid + ` <br/><br/>

如果这个请求不是由您发起的，那没问题，您不用担心，您可以安全地忽略这封邮件。<br/><br/>

如果您有任何疑问，可以回复这封邮件向我们提问。谢谢！<br/><br/>

<div style="text-align:right;">&copy;2013 studygolang.com  Golang中文社区 | Go语言学习园地</div>`
	service.SendMail("【Golang中文社区】重设密码 ", content, []string{email})
}

func setCookie(rw http.ResponseWriter, req *http.Request, username string) {
	session, _ := filter.Store.Get(req, "user")
	if req.FormValue("remember_me") != "1" {
		// 浏览器关闭，cookie删除，否则保存30天
		session.Options = &sessions.Options{
			Path: "/",
		}
	}
	session.Values["username"] = username
	session.Save(req, rw)

}

// 注销
// uri : /account/logout
func LogoutHandler(rw http.ResponseWriter, req *http.Request) {
	// 删除cookie信息
	session, _ := filter.Store.Get(req, "user")
	session.Options = &sessions.Options{Path: "/", MaxAge: -1}
	session.Save(req, rw)
	// 重定向得到登录页（TODO:重定向到什么页面比较好？）
	util.Redirect(rw, req, "/account/login")
}
