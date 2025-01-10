package build

import (
	"bytes"
	"crypto/md5"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/desertthunder/documango/cmd/config"
	"github.com/desertthunder/documango/libs"
)

//go:embed assets/theme.js
var ScriptFile string

type FilePath struct {
	FileP string
	Name  string
}

type Builder struct {
	Config *config.Config
	Logger *log.Logger
	state  *builderState
}

type builderState struct {
	static bool
	view   bool
	asset  bool
	errors []error
}

var failBuildAndExit func(msg string) = func(msg string) {
	BuildLogger.Fatal(msg)
}

func createStaticBuildDir(c *config.Config) string {
	dest := libs.CreateDir(c.Options.BuildDir + "/assets")
	BuildLogger.Debugf("created directory %v", dest)
	return dest
}

// CopyStaticFiles creates the build dir at d, the provided destination
// directory as well as the static files directory at {dest}/assets
func CopyStaticFiles(c *config.Config) ([]*FilePath, error) {
	paths := []*FilePath{}
	src := c.Options.StaticDir
	dest := createStaticBuildDir(c)
	entries, err := os.ReadDir(src)
	if err != nil {
		return paths, fmt.Errorf("unable to read directory %v %v", src, err.Error())
	}

	for _, entry := range entries {
		fname := entry.Name()
		if entry.IsDir() {
			continue
		}

		path, _ := libs.CopyFile(fname, src, dest)
		paths = append(paths, &FilePath{path, fname})
	}

	theme := BuildTheme()
	theme_path := fmt.Sprintf("%v/styles.css", dest)
	err = libs.CreateAndWriteFile([]byte(theme), theme_path)

	if err != nil {
		BuildLogger.Warnf("unable to write theme to %v/styles.css \n%v", dest, err.Error())
		return paths, nil
	} else {
		paths = append(paths, &FilePath{Name: "styles.css", FileP: theme_path})
	}

	return paths, nil
}

func CollectStatic(c *config.Config) ([]*FilePath, error) {
	b := c.Options.BuildDir
	defer BuildLogger.Infof("copied static files from %v to %v", c.Options.StaticDir, b)
	static_paths, err := CopyStaticFiles(c)

	if err != nil {
		BuildLogger.Warnf("collecting static files failed: %v", err.Error())

		_ = CopyJS(c)
	}

	theme := BuildTheme()
	// The failure case here is when the file exists but that is handled by CopyFile
	libs.CreateAndWriteFile([]byte(theme), fmt.Sprintf("%v/assets/styles.css", b))

	return static_paths, err
}

// When using the default template, {views}/base, we want to bundle assets/theme.js
// to ensure that the user can access the basic light/dark toggler.
//
// TODO: this should be configurable
func CopyJS(conf *config.Config) error {
	fs, err := os.Stat(conf.Options.TemplateDir)

	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			fpath := fmt.Sprintf("%v/assets/theme.js", conf.Options.BuildDir)
			f, err := os.Create(fpath)

			if err != nil {
				BuildLogger.Errorf("sww %v", err.Error())
				return err
			}

			if _, err = f.Write([]byte(ScriptFile)); err != nil {
				BuildLogger.Errorf("sww %v", err.Error())
				return err
			}

			BuildLogger.Info("copied theme.js to /dist/")
			return nil
		} else {
			BuildLogger.Errorf("sww %v", err.Error())
			return err
		}
	}

	if fs.IsDir() {
		BuildLogger.Info("template directory present, using custom theme")
		return nil
	}

	return nil
}

/*
Builder actions

With the exception of the template execution/view building action, we want to be able to
recover from errors & panics.

Each of these is a goroutine.
*/

func initialBuilderState() *builderState {
	return &builderState{static: false, view: false, asset: false}
}

// function SetupBuilder is the Builder struct constructor
// It handles preflight checks
// (TODO) copies an existing build dir to from {name} to _name
// (TODO) versioning builds?
func SetupBuilder(c *config.Config, l *log.Logger) *Builder {
	if l == nil {
		return &Builder{Config: c, Logger: BuildLogger, state: initialBuilderState()}
	} else {
		return &Builder{Config: c, Logger: l}
	}
}

func (b Builder) BuildDir() string {
	return b.Config.Options.BuildDir
}

func (b Builder) StaticDir() string {
	return b.Config.Options.StaticDir
}

// function BuildStatic copies static files to the build directory
//  1. Checks that a static dir (defined in conf struct) exists
//  2. Check that a build static dir exists and is not empty
//  3. Reads the list of files in the static dir and generate hashes for each file's contents
//  4. Does the same for the old static files, and compares hashes. If they are different, replace.
//
// Can recover
func (b *Builder) BuildStatic() {
	old_build_dir := fmt.Sprintf("_%v/assets", b.BuildDir())
	new_build_dir := b.StaticDir()
	// TODO: just straight copy old if is a new build
	current_build_dir := fmt.Sprintf("%v/assets", b.BuildDir())

	old_file_info, err := os.ReadDir(old_build_dir)
	b.state.errors = append(b.state.errors, err)

	new_file_info, err := os.ReadDir(new_build_dir)
	b.state.errors = append(b.state.errors, err)

	for _, new_file_entry := range new_file_info {
		new_file_name := new_file_entry.Name()

		var old_file_entry fs.DirEntry
		for _, _f := range old_file_info {
			if _f.Name() == new_file_name {
				old_file_entry = _f
				break
			}
		}

		if old_file_entry == nil {
			continue
		}

		new_file_hash := md5.New()
		new_file_contents, err := os.ReadFile(fmt.Sprintf("%v/%v", new_build_dir, new_file_name))
		b.state.errors = append(b.state.errors, err)

		reader := bytes.NewReader(new_file_contents)

		_, err = io.Copy(new_file_hash, reader)
		b.state.errors = append(b.state.errors, err)

		old_file_hash := md5.New()
		old_file_contents, err := os.ReadFile(fmt.Sprintf("%v/%v", old_build_dir, old_file_entry.Name()))
		b.state.errors = append(b.state.errors, err)

		reader.Reset(old_file_contents)

		_, err = io.Copy(new_file_hash, reader)
		b.state.errors = append(b.state.errors, err)

		if new_file_hash == old_file_hash {
			continue
		}

		err = os.Remove(fmt.Sprintf("%v/%v", current_build_dir, old_file_entry.Name()))
		b.state.errors = append(b.state.errors, err)

		new_file, err := os.Create(fmt.Sprintf("%v/%v", current_build_dir, new_file_entry.Name()))
		b.state.errors = append(b.state.errors, err)

		_, err = new_file.Write(new_file_contents)
		b.state.errors = append(b.state.errors, err)
	}
}

func (b *Builder) rmNilErrors() {
	// Remove nil errors
	final_error_set := []error{}
	for _, e := range b.state.errors {
		if e == nil {
			continue
		} else {
			final_error_set = append(final_error_set, e)
		}
	}

	b.state.errors = final_error_set
}

// An error here *should* end execution
func (b *Builder) BuildViews() {}
