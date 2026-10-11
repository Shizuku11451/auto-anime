package main

import "sync"

// Configuration writes publish under this lock; readers take a value copy and
// release it before network or database I/O. Legacy globals remain startup defaults.
var runtimeConfigMu sync.RWMutex

type runtimeSettings struct {
	downloadMode            string
	aria2URL                string
	aria2Token              string
	aria2Dir                string
	aria2LocalDir           string
	qbitURL                 string
	qbitUser                string
	qbitPassword            string
	qbitSavePath            string
	qbitLocalDir            string
	transmissionURL         string
	transmissionUser        string
	transmissionPassword    string
	transmissionDownloadDir string
	transmissionLocalDir    string
	keepSourceFiles         bool
	pollMinutes             int
	libraryDir              string
	webUser                 string
	webHash                 string
	webPassword             string
	setupDone               bool
	discoverSearchTemplate  string
}

func runtimeConfig() runtimeSettings {
	runtimeConfigMu.RLock()
	defer runtimeConfigMu.RUnlock()
	return runtimeSettings{
		downloadMode:            downloadMode,
		aria2URL:                aria2URL,
		aria2Token:              aria2Token,
		aria2Dir:                aria2Dir,
		aria2LocalDir:           aria2LocalDir,
		qbitURL:                 qbitURL,
		qbitUser:                qbitUser,
		qbitPassword:            qbitPassword,
		qbitSavePath:            qbitSavePath,
		qbitLocalDir:            qbitLocalDir,
		transmissionURL:         transmissionURL,
		transmissionUser:        transmissionUser,
		transmissionPassword:    transmissionPassword,
		transmissionDownloadDir: transmissionDownloadDir,
		transmissionLocalDir:    transmissionLocalDir,
		keepSourceFiles:         keepSourceFiles,
		pollMinutes:             pollMinutes,
		libraryDir:              libraryDir,
		webUser:                 webUser,
		webHash:                 webHash,
		webPassword:             webPassword,
		setupDone:               setupDone,
		discoverSearchTemplate:  discoverSearchTemplate,
	}
}
