package commands

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"sync"

	"strings"

	"github.com/davinche/bro/concurrent"
	"github.com/urfave/cli"
)

func getBroConfigDir() (string, error) {
	cuser, err := user.Current()
	if err != nil {
		return "", err
	}

	configHome := filepath.Join(cuser.HomeDir, ".bros")
	if _, err = os.Stat(configHome); os.IsNotExist(err) {
		if err = os.Mkdir(configHome, os.ModePerm); err != nil {
			return "", err
		}
	}
	return configHome, nil
}

func createNewProject(name string) error {
	broDir, err := getBroConfigDir()
	if err != nil {
		return err
	}

	projectDir := filepath.Join(broDir, name)

	// make sure folder does not exist and there are no "other" errors
	info, err := os.Stat(projectDir)
	if err == nil && info.IsDir() {
		return fmt.Errorf("project with the name %q already exists", name)
	}

	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// create the project
		err = os.Mkdir(projectDir, os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
}

func getExistingProject(name string) (string, error) {
	broDir, err := getBroConfigDir()
	if err != nil {
		return "", err
	}

	p := filepath.Join(broDir, name)
	info, err := os.Stat(filepath.Join(broDir, name))
	if err == nil && info.IsDir() {
		return p, nil
	}

	if err != nil && os.IsNotExist(err) {
		return "", fmt.Errorf("Project %q does not exist", name)
	}

	if err != nil {
		return "", err
	}

	return "", nil
}

// ----------------------------------------------------------------------------
// Current Project Stuff ------------------------------------------------------
// ----------------------------------------------------------------------------

func getRootBro() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for dir != "/" {
		check := filepath.Join(dir, ".bro")
		if info, err := os.Stat(check); err == nil {
			if info.IsDir() {
				return dir, nil
			}
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("not a bro project")
}

func getBroConfig() (*Brofiguration, error) {
	dir, err := getRootBro()
	if err != nil {
		return nil, err
	}

	configFile := filepath.Join(dir, ".bro", "bro.json")
	_, err = os.Stat(configFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		// make the file
		f, err := os.Create(configFile)
		if err != nil {
			return nil, err
		}
		enc := json.NewEncoder(f)
		if err = enc.Encode(Brofiguration{}); err != nil {
			return nil, err
		}
		return &Brofiguration{}, nil
	}

	config := Brofiguration{}
	raw, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil

}

func initializeBroDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, ".bro")
	if err = os.MkdirAll(p, os.ModePerm); err != nil {
		return "", err
	}
	return p, nil
}

func resetStage() error {
	broDir, err := getRootBro()
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(broDir, ".bro", "_stage"))
}

// Brofiguration is the configuration used to determine which template
// the current project is mapped to
type Brofiguration struct {
	Bro string `json:"bro"`
}

// Create generates a new project
func Create(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("Please specify a project name to create")
	}

	name := strings.Trim(c.Args().First(), " ")
	if err := createNewProject(name); err != nil {
		return err
	}

	if err := Init(c); err != nil {
		return err
	}

	fmt.Printf("Successfully created project %q.\n", name)
	if err := Track(c); err != nil {
		return err
	}
	return nil
}

// Init takes a folder and creates a new scaffold based off of the folder
func Init(c *cli.Context) error {
	// get the current working directory to init the bro project
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	// make the .bro directory
	broPath := filepath.Join(dir, ".bro")
	if _, err = os.Stat(broPath); os.IsNotExist(err) {
		if err = os.Mkdir(broPath, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

// Add takes files and directories specified in the command line args
// and adds them into the working directory
func Add(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("one or more files must be specified")
	}

	broRoot, err := getRootBro()
	if err != nil {
		return err
	}

	// collect the files
	files := sync.Map{}
	walker := concurrent.NewCWalker(c.GlobalInt("threads"))
	for _, f := range c.Args() {
		p, err := filepath.Abs(f)
		if err != nil {
			return err
		}

		fileInfo, err := os.Stat(p)
		if err != nil {
			return err
		}

		if fileInfo.IsDir() {
			walker.WalkAndCollect(p, &files)
		} else {
			files.Store(p, p)
		}
	}

	// copy the files
	broDir := filepath.Join(broRoot, ".bro")
	stagePath := filepath.Join(broDir, "_stage")

	copier := concurrent.NewCCopier(c.GlobalInt("threads"))
	copier.Start()
	files.Range(func(_, v interface{}) bool {
		if sv, ok := v.(string); ok {
			if strings.HasPrefix(sv, broDir) {
				return true
			}
			dst := filepath.Join(stagePath, strings.TrimPrefix(sv, broRoot))
			copier.Copy(sv, dst)
		}
		return true
	})
	copier.Wait()
	return nil
}

// Reset takes a file or directory and removes all of them from staging
func Reset(c *cli.Context) error {
	if err := resetStage(); err != nil {
		return err
	}
	fmt.Println("Staged files removed.")
	return nil
}

// Commit saves the staged files into the bundle
func Commit(c *cli.Context) error {
	broRoot, err := getRootBro()
	if err != nil {
		return err
	}

	config, err := getBroConfig()
	if err != nil {
		return err
	}

	if config.Bro == "" {
		return fmt.Errorf("Cannot commit. Project not tracked")
	}

	configDir, err := getExistingProject(config.Bro)
	if err != nil {
		return err
	}

	stage := filepath.Join(broRoot, ".bro", "_stage")

	// collect all files in stage
	files := sync.Map{}
	walker := concurrent.NewCWalker(c.GlobalInt("threads"))
	walker.WalkAndCollect(stage, &files)

	files.Range(func(k, v interface{}) bool {
		if p, ok := v.(string); ok {
			f := strings.TrimPrefix(p, stage)
			dst := filepath.Join(configDir, f)
			_, err := os.Stat(dst)

			// remove the file if it exists
			if err == nil {
				err = os.Remove(dst)
				if err != nil {
					fmt.Println("whopesee")
					return false
				}
			}

			if err != nil && !os.IsNotExist(err) {
				fmt.Println("stat whopsee")
				return false
			}

			// make sure folder exist
			destDir := filepath.Dir(dst)
			err = os.MkdirAll(destDir, os.ModePerm)
			if err != nil {
				fmt.Println("Could not make folder path.")
				return false
			}

			// move the file
			// fmt.Println("copying:  -------")
			// fmt.Println(p)
			// fmt.Println(dst)
			err = os.Rename(p, dst)
			if err != nil {
				fmt.Println("rename whopesee")
				return false
			}
			return true
		}
		fmt.Println("uhh..")
		return false
	})

	fmt.Println("Files commited.")
	// commit all staging to bundle
	return nil
}

// Status provides information on the currently staged items
func Status(c *cli.Context) error {
	broRoot, err := getRootBro()
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// check for config
	config, err := getBroConfig()
	if err != nil {
		return err
	}

	if config.Bro != "" {
		fmt.Printf("Tracked against %q:\n", config.Bro)
	} else {
		fmt.Println("Currently untracked:")
	}
	fmt.Printf("  use \"bro track <project>\" to track against another project\n")
	fmt.Println()

	// get staged files
	files := sync.Map{}
	stagePath := filepath.Join(broRoot, ".bro", "_stage")

	// check if there is anything to stage
	if _, err = os.Stat(stagePath); os.IsNotExist(err) {
		fmt.Println("No files to be commited.")
		return nil
	}

	// walk the stage
	walker := concurrent.NewCWalker(c.GlobalInt("threads"))
	walker.WalkAndCollect(stagePath, &files)

	// Print path
	fmt.Println("Files to commit to template:")
	fmt.Println("  (use \"bro reset\" to remove all files from staging)")
	fmt.Println()
	files.Range(func(_, v interface{}) bool {
		if sv, ok := v.(string); ok {
			p := filepath.Join(broRoot, strings.TrimPrefix(sv, stagePath))
			rel, err := filepath.Rel(wd, p)
			if err != nil {
				fmt.Println(err)
				return false
			}
			if !strings.HasPrefix(rel, ".") {
				rel = "." + string(filepath.Separator) + rel
			}
			fmt.Printf("        %s\n", rel)
		}
		return true
	})
	return nil
}

// Track takes the current project and tracks it against an existing template
func Track(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("Track error: name required")
	}

	broConfigDir, err := getBroConfigDir()
	if err != nil {
		return err
	}

	// make sure it exists
	name := strings.Trim(c.Args().First(), " ")
	_, err = os.Stat(filepath.Join(broConfigDir, name))

	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Track error: project does not exist")
		}
		return err
	}

	// make sure the .bro directory is created
	dir, err := initializeBroDir()
	if err != nil {
		return err
	}

	// marshal the json
	brofiguration := Brofiguration{name}
	marshalled, err := json.Marshal(brofiguration)
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(filepath.Join(dir, "bro.json"), marshalled, os.ModePerm); err != nil {
		return err
	}

	fmt.Printf("Tracking against %q:\n", name)
	return nil
}

// Clone clones a project into a specified folder
func Clone(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("Please specify a project name to clone.")
	}

	projName := c.Args().First()
	configDir, err := getExistingProject(projName)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// destinationDir
	dstDir := filepath.Join(wd, projName)
	if c.NArg() > 1 {
		dstDir = filepath.Join(wd, c.Args().Get(1))
	}

	files := sync.Map{}
	walker := concurrent.NewCWalker(c.GlobalInt("threads"))
	walker.WalkAndCollect(configDir, &files)

	// copy to destination
	copier := concurrent.NewCCopier(c.GlobalInt("threads"))
	copier.Start()
	files.Range(func(_, v interface{}) bool {
		if sv, ok := v.(string); ok {
			dst := filepath.Join(dstDir, strings.TrimPrefix(sv, configDir))
			copier.Copy(sv, dst)
		}
		return true
	})
	copier.Wait()
	fmt.Printf("Successfully cloned %q.\n", projName)
	return nil
}
