package blender

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Client struct {
	blenderPath string
	workspace   string
}

type Config struct {
	BlenderPath string
	Workspace   string
}

func NewClient(cfg Config) *Client {
	path := cfg.BlenderPath
	if path == "" {
		path = "blender"
	}
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = "."
	}

	return &Client{
		blenderPath: path,
		workspace:   workspace,
	}
}

type Scene struct {
	Name        string       `json:"name"`
	Objects     []Object     `json:"objects"`
	Collections []Collection `json:"collections"`
}

type Object struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Location []float64 `json:"location"`
	Rotation []float64 `json:"rotation"`
	Scale    []float64 `json:"scale"`
}

type Collection struct {
	Name     string   `json:"name"`
	Children []string `json:"children,omitempty"`
}

type RenderResult struct {
	OutputPath string `json:"output_path"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Engine     string `json:"engine"`
	Duration   string `json:"duration"`
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.listScenes(ctx)
	}

	cmd := args[0]
	switch cmd {
	case "list", "ls":
		return c.listScenes(ctx)
	case "scene", "new":
		return c.createScene(ctx, args[1:])
	case "object", "add":
		return c.addObject(ctx, args[1:])
	case "render", "export":
		return c.render(ctx, args[1:])
	case "info":
		return c.info(ctx)
	default:
		return c.runPython(ctx, strings.Join(args, " "))
	}
}

func (c *Client) listScenes(ctx context.Context) (string, error) {
	script := `
import bpy
for scene in bpy.data.scenes:
    print(scene.name)
`
	return c.runScript(ctx, script)
}

func (c *Client) createScene(ctx context.Context, args []string) (string, error) {
	name := "NewScene"
	if len(args) > 0 {
		name = args[0]
	}

	script := fmt.Sprintf(`
import bpy
scene = bpy.data.scenes.new(name="%s")
bpy.context.window.scene = scene
print("Created scene:", "%s")
`, name, name)

	out, err := c.runScript(ctx, script)
	if err != nil {
		return "", err
	}
	return out + "\nScene created: " + name, nil
}

func (c *Client) addObject(ctx context.Context, args []string) (string, error) {
	objType := "cube"
	objName := "Object"
	if len(args) > 0 {
		objType = args[0]
	}
	if len(args) > 1 {
		objName = args[1]
	}

	script := fmt.Sprintf(`
import bpy
obj_type = "%s"
name = "%s"
if obj_type == "cube":
    bpy.ops.mesh.primitive_cube_add(size=2, location=(0, 0, 0))
    obj = bpy.context.active_object
elif obj_type == "sphere":
    bpy.ops.mesh.primitive_uv_sphere_add(radius=1, location=(0, 0, 0))
    obj = bpy.context.active_object
elif obj_type == "plane":
    bpy.ops.mesh.primitive_plane_add(size=2, location=(0, 0, 0))
    obj = bpy.context.active_object
elif obj_type == "cylinder":
    bpy.ops.mesh.primitive_cylinder_add(radius=1, depth=2, location=(0, 0, 0))
    obj = bpy.context.active_object
elif obj_type == "cone":
    bpy.ops.mesh.primitive_cone_add(radius1=1, depth=2, location=(0, 0, 0))
    obj = bpy.context.active_object
else:
    bpy.ops.mesh.primitive_cube_add(size=2, location=(0, 0, 0))
    obj = bpy.context.active_object

obj.name = name
print("Added object:", name, "type:", obj_type)
`, objType, objName)

	return c.runScript(ctx, script)
}

func (c *Client) render(ctx context.Context, args []string) (string, error) {
	outputPath := "render.png"
	format := "PNG"
	if len(args) > 0 {
		outputPath = args[0]
	}
	if len(args) > 1 {
		format = args[1]
	}

	absPath, _ := filepath.Abs(outputPath)

	script := fmt.Sprintf(`
import bpy
scene = bpy.context.scene
scene.render.filepath = "%s"
scene.render.image_settings.file_format = "%s"
bpy.ops.render.render(write=True)
print("Rendered to:", "%s")
`, outputPath, format, absPath)

	_, err := c.runScript(ctx, script)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Rendered: %s", absPath), nil
}

func (c *Client) info(ctx context.Context) (string, error) {
	script := `
import bpy
scene = bpy.context.scene
print("Scene:", scene.name)
print("Objects:", len(bpy.data.objects))
print("Collections:", len(bpy.data.collections))
for obj in bpy.data.objects[:5]:
    print(" -", obj.name, "type:", obj.type)
`
	return c.runScript(ctx, script)
}

func (c *Client) runPython(ctx context.Context, code string) (string, error) {
	script := code
	return c.runScript(ctx, script)
}

func (c *Client) runScript(ctx context.Context, script string) (string, error) {
	tmpfile, err := os.CreateTemp("", "blender_*.py")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(script); err != nil {
		return "", err
	}
	tmpfile.Close()

	cmd := exec.CommandContext(ctx, c.blenderPath, "--background", "--python", tmpfile.Name())
	cmd.Dir = c.workspace
	output, err := cmd.CombinedOutput()

	if err != nil {
		return string(output), err
	}

	lines := strings.Split(string(output), "\n")
	var result []string
	for _, line := range lines {
		if !strings.HasPrefix(line, "Read prefs:") &&
			!strings.HasPrefix(line, "found bundled Python:") &&
			!strings.HasPrefix(line, "Warning:") {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n"), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.blenderPath, "--version")
	return cmd.Run() == nil
}
