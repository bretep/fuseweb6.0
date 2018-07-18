package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strings"

	"unicode/utf8"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
	"bazil.org/fuse/fuseutil"
	_ "github.com/lib/pq"
	"golang.org/x/net/context"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", os.Args[0])
	flag.PrintDefaults()
}

var configOpts web6Config

func loadConfig(config string) {
	_, err := os.Stat(config)
	if err != nil {
		usr, _ := user.Current()
		homedir := usr.HomeDir
		config = fmt.Sprintf("%s/secure/web6.json", homedir)
		_, err := os.Stat(config)
		if err != nil {
			panic(fmt.Sprintf("Could not find web6.json in %s", config))
		}
	}

	fmt.Printf("Load web6 config: %s\n", config)

	configString, err := ioutil.ReadFile(config)
	if err != nil {
		log.Panic(fmt.Sprintf("Cannot read config file: %s: %s\n", config, err.Error()))
	}

	err = json.Unmarshal([]byte(configString), &configOpts)
	if err != nil {
		log.Panic(fmt.Sprintf("Cannot parse JSON config file: %s: %s\n", config, err.Error()))
	}
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func run(mountpoint string) error {

	connInfo := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable",
		configOpts.DefaultDatabase.User,
		configOpts.DefaultDatabase.Password,
		configOpts.DefaultDatabase.Database)
	db, err := sql.Open("postgres", connInfo)
	checkErr(err)
	defer db.Close()

	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName("fuseweb"),
		fuse.Subtype("fusewebfs"),
		fuse.LocalVolume(),
		fuse.VolumeName("Fuse Web 6.0 Templates"),
	)
	if err != nil {
		return err
	}
	defer c.Close()

	if p := c.Protocol(); !p.HasInvalidate() {
		return fmt.Errorf("kernel FUSE support is too old to have invalidations: version %v", p)
	}

	srv := fs.New(c, nil)
	filesys := &FS{
		database: db,
	}

	if err := srv.Serve(filesys); err != nil {
		return err
	}

	// Check if the mount process has an error to report.
	<-c.Ready
	if err := c.MountError; err != nil {
		return err
	}
	return nil
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}
	mountpoint := flag.Arg(0)

	loadConfig("")

	if err := run(mountpoint); err != nil {
		log.Fatal(err)
	}
}

var _ fs.FS = (*FS)(nil)

func (f *FS) Root() (fs.Node, error) {
	return &Dir{
		database: f.database,
	}, nil
}

var _ fs.Node = (*Dir)(nil)

func (d *Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Mode = os.ModeDir | 0755
	return nil
}

var _ fs.NodeRequestLookuper = (*Dir)(nil)

func (d *Dir) Lookup(ctx context.Context, req *fuse.LookupRequest, resp *fuse.LookupResponse) (fs.Node, error) {
	prefix := req.Name
	if d.file != nil {
		prefix = fmt.Sprintf("%s/%s", d.file.FusePathName, prefix)
	}
	var prefixExists bool
	prefixExists = false
	for _, row := range returnFiles(d.database) {
		pRow := &row
		var tempName = fmt.Sprintf("%d/%s", row.ThemeId.Int64, row.Path.String)
		if prefix == tempName {
			pRow.FusePathName = prefix
			return &File{
				file:     pRow,
				database: d.database,
			}, nil
		}

		if strings.HasPrefix(tempName, prefix) {
			prefixExists = true
		}
	}

	if prefixExists {
		return &Dir{
			database: d.database,
			file: &dbFile{
				FusePathName: prefix,
			},
		}, nil
	}

	return nil, fuse.ENOENT
}

var _ fs.HandleReadDirAller = (*Dir)(nil)

func (d *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	prefix := ""
	if d.file != nil {
		prefix = d.file.FusePathName
	}

	dirList := make(map[string]bool)
	var res []fuse.Dirent
	for _, row := range returnFiles(d.database) {
		var tempName = fmt.Sprintf("%d/%s", row.ThemeId.Int64, row.Path.String)
		var dirEnt = ""
		var dirEntTemp []string
		var de fuse.Dirent
		if strings.HasPrefix(tempName, prefix) {
			// Root of directory
			if prefix == "" {
				dirEnt = strings.Split(tempName, "/")[0]
			} else {
				// Add directory separator to prefix
				prefixSplit := prefix + "/"
				// Split the current row string
				dirEntTemp = strings.Split(tempName, prefixSplit)
				// If result contains a directory separator then, we're in a directory
				if len(dirEntTemp) <= 1 {
					// Cannot split, so this file should not be in here
					continue
				}
				if strings.ContainsRune(dirEntTemp[1], '/') {
					dirEnt = strings.Split(dirEntTemp[1], "/")[0]
				} else {
					dirEntTemp = strings.Split(tempName, "/")
					dirEnt = dirEntTemp[len(dirEntTemp)-1]
					dirList[dirEnt] = true
					de.Name = dirEnt
					res = append(res, de)
					continue
				}
			}
			if _, value := dirList[dirEnt]; !value {
				dirList[dirEnt] = true
				de.Type = fuse.DT_Dir
				de.Name = dirEnt
				res = append(res, de)
			} else {
				continue
			}
		}
	}
	return res, nil
}

var _ fs.Node = (*File)(nil)

func fileAttr(f *dbFile, a *fuse.Attr) {
	a.Size = uint64(utf8.RuneCountInString(f.Html.String))
	a.Inode = uint64(f.Id.Int64)
	a.Mode = 0666
}

func (f *File) Attr(ctx context.Context, a *fuse.Attr) error {
	fileAttr(f.file, a)
	return nil
}

var _ fs.NodeOpener = (*File)(nil)

func (f *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	resp.Flags |= fuse.OpenKeepCache

	return f, nil
}

var _ fs.Handle = (*File)(nil)

var _ fs.HandleReader = (*File)(nil)

func (f *File) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	t := f.file.Html.String
	fuseutil.HandleRead(req, resp, []byte(t))
	return nil
}

func (f *File) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	resp.Size = len(req.Data)
	fileContents, err := writeFile(f.database, req.Data, f.file.Id.Int64, req.Offset)
	checkErr(err)
	f.file.Html = sql.NullString{
		String: fileContents,
		Valid:  true,
	}
	return nil
}

func (f *File) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	return nil
}
