package output

import (
	"encoding/json"
	"fmt"
)

// JSON imprime v como JSON indentado — usado quando --output json.
func JSON(v interface{}) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
