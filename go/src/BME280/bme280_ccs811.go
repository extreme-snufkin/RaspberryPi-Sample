package main

import (
	"container/list"
	"database/sql"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"gobot.io/x/gobot"
	"gobot.io/x/gobot/drivers/i2c"
	"gobot.io/x/gobot/platforms/raspi"
)

const (
	//初期設定定数
	logPath             = "/home/macho/log/bme280_sensor.log"
	layout              = "2006-01-02 15:04"
	dbpath              = "/home/macho/sqlite/bme280_sensor.db"
	sqlSelectALERT      = "SELECT CONDITION FROM ALERT WHERE CODE = '"
	sqlUpdateALERT      = "UPDATE ALERT SET CONDITION=?, UPD_DATETIME_TEXT=? where CODE=?"
	sqlInsertBME280     = "INSERT INTO BME280(PRESSURE, TEMPERATURE, HUMIDITY, INS_DATETIME_TEXT, INS_DATETIME_INTEGER) VALUES (?, ?, ?, ?, ?)"
	sqlSelectBME280     = "SELECT PRESSURE, INS_DATETIME_TEXT FROM BME280 WHERE INS_DATETIME_TEXT = '"
	sqlSelectPUSH       = "SELECT TOKEN FROM PUSH"
	depCode             = "DEP"
	alertOn             = "true"
	alertOff            = "false"
	beforeHour          = -3 //3時間前
	lp                  = 4
	pushShell           = "/home/macho/go/src/BME280/push.sh"
	pushMessage         = " 気圧が急激に低下しています。"
	pushStopMessage     = " 気圧の急激な低下が治まりました。"
	nonPushMessage      = " 急激な気圧低下なし。"
	nonPushGoingMessage = " 急激な気圧低下継続中。"
	ccs811Python        = "/home/macho/python/Adafruit_CCS811_python/src/CCS811.py"
)

func main() {
	// ログ出力設定
	logFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	defer func() {
		logFile.Close()
	}()
	log.SetOutput(logFile)

	r := raspi.NewAdaptor()
	bme280 := i2c.NewBME280Driver(r, i2c.WithAddress(0x76))

	work := func() {
		sensorPressure, err := bme280.Pressure()
		if err != nil {
			log.Print(err)
		}

		sensorTemperature, err := bme280.Temperature()
		if err != nil {
			log.Print(err)
		}

		sensorHumidity, err := bme280.Humidity()
		if err != nil {
			log.Print(err)
		}

		bme280Insert(float64(sensorPressure), float64(sensorTemperature), float64(sensorHumidity))
		dbInsDatetime, dbPressure := bme280Select()
		var _ = dbInsDatetime
		// CCS811を呼び出す予定
		checkPush(dbPressure, float64(sensorPressure))

		os.Exit(0)
	}

	robot := gobot.NewRobot("bme280bot",
		[]gobot.Connection{r},
		[]gobot.Device{bme280},
		work,
	)

	err := robot.Start()
	if err != nil {
		log.Print(err)
	}
}

func bme280Insert(pressure float64, temperature float64, humidity float64) {
	day := time.Now()
	db, err := sql.Open("sqlite3", dbpath)
	checkErr(err)
	//データの新規登録
	stmt, err := db.Prepare(sqlInsertBME280)
	checkErr(err)
	res, err := stmt.Exec(pressure, temperature, humidity, day.Format(layout), day.Unix())
	checkErr(err)
	var _ = res
	db.Close()
}

func bme280Select() (string, float64) {
	var pressure float64
	var insDatetime string
	var resPressure float64
	var resInsDatetime string
	day := time.Now()
	h3 := day.Add(beforeHour * time.Hour) // 3時間前

	db, err := sql.Open("sqlite3", dbpath)
	checkErr(err)
	//データの検索
	rows, err := db.Query(sqlSelectBME280 + h3.Format(layout) + "'")
	checkErr(err)
	for rows.Next() {
		err = rows.Scan(&pressure, &insDatetime)
		checkErr(err)
		resInsDatetime = insDatetime
		resPressure = pressure
	}
	db.Close()
	// log.Print(resInsDatetime + " _ " + strconv.FormatFloat(Round(pressure/100.0, 2), 'f', 2, 64) + "hpa")
	return resInsDatetime, resPressure
}

func alertUpdate(condition string, depCode string) {
	day := time.Now()
	db, err := sql.Open("sqlite3", dbpath)
	checkErr(err)
	//データの更新
	stmt, err := db.Prepare(sqlUpdateALERT)
	checkErr(err)
	res, err := stmt.Exec(condition, day.Format(layout), depCode)
	checkErr(err)
	var _ = res
	db.Close()
}

func alertSelect(depCode string) string {
	var condition string
	var resContdition string
	db, err := sql.Open("sqlite3", dbpath)
	checkErr(err)
	//データの検索
	rows, err := db.Query(sqlSelectALERT + depCode + "'")
	checkErr(err)
	for rows.Next() {
		err = rows.Scan(&condition)
		checkErr(err)
		resContdition = condition
	}
	db.Close()
	return resContdition
}

func pushSelect(resTokenList *list.List) *list.List {
	db, err := sql.Open("sqlite3", dbpath)
	checkErr(err)
	//データの検索
	rows, err := db.Query(sqlSelectPUSH)
	checkErr(err)
	for rows.Next() {
		var token string
		err = rows.Scan(&token)
		checkErr(err)
		resTokenList.PushBack(token)
	}
	db.Close()
	return resTokenList
}

func checkPush(pastPressure float64, nowPressure float64) {
	day := time.Now()
	initList := list.New()
	tokenList := list.New()
	dbCondition := alertSelect(depCode)
	if (pastPressure/100.0)-(nowPressure/100.0) >= lp {
		if dbCondition == "false" {
			var cmdArgs string
			tokenList = pushSelect(initList)
			for element := tokenList.Front(); element != nil; element = element.Next() {
				cmdArgs = day.Format(layout) + pushMessage + "(" + strconv.FormatFloat(Round(nowPressure/100.0, 2), 'f', 2, 64) + "hpa)"
				err := exec.Command("/bin/sh", pushShell, element.Value.(string), cmdArgs).Run()
				checkErr(err)
			}
			alertUpdate(alertOn, depCode)
			log.Print(pushMessage)
		} else {
			log.Print(nonPushGoingMessage)
		}
	} else {
		if dbCondition == "true" {
			var cmdArgs string
			tokenList = pushSelect(initList)
			for element := tokenList.Front(); element != nil; element = element.Next() {
				cmdArgs = day.Format(layout) + pushStopMessage + "(" + strconv.FormatFloat(Round(nowPressure/100.0, 2), 'f', 2, 64) + "hpa)"
				err := exec.Command("/bin/sh", pushShell, element.Value.(string), cmdArgs).Run()
				checkErr(err)
			}
			alertUpdate(alertOff, depCode)
			log.Print(pushStopMessage)
		} else {
			log.Print(nonPushMessage)
		}
	}
}

func checkErr(err error) {
	if err != nil {
		log.Print(err)
		panic(err)
	}
}

func Round(f float64, places int) float64 {
	shift := math.Pow(10, float64(places))
	return math.Floor(f*shift+.5) / shift
}
