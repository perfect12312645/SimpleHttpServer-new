package utils

import "fmt"

func FormatSize(size int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	floatSize := float64(size)

	for floatSize >= 1024 && unitIndex < len(units)-1 {
		floatSize /= 1024
		unitIndex++
	}

	return fmt.Sprintf("%.2f %s", floatSize, units[unitIndex])
}
