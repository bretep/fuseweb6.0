package main

import (
	"database/sql"
)

func writeFile(d *sql.DB, f []byte, id int64, ro int64) (string, error) {
	var html sql.NullString
	toWrite := string(f)
	updateStmt := `
UPDATE web_widget
SET html = $2
WHERE _id = $1;`

	getStmt := `
SELECT html 
FROM web_widget
WHERE _id = $1;`
	if ro > 0 {
		oFile := d.QueryRow(getStmt, id)
		oFile.Scan(&html)
		toWrite = html.String + string(f)
	}

	_, err := d.Exec(updateStmt, id, []byte(toWrite))
	if err != nil {
		return "", err
	}

	return toWrite, nil
}

func returnFiles(d *sql.DB) []dbFile {
	var res []dbFile
	rows, err := d.Query("SELECT * FROM web_widget")
	checkErr(err)
	for rows.Next() {
		var row = dbFile{}
		err = rows.Scan(
			&row.Id,
			&row.ThemeId,
			&row.TypeId,
			&row.Name,
			&row.Path,
			&row.Info,
			&row.Html,
			&row.UdnJson)
		checkErr(err)
		res = append(res, row)
	}
	return res
}
