package ldb

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

func (t *TableDefinition) Query(keyHex string, dataChan chan []string, closeChan bool) (int, error) {

	count := 0

	var cmdStr strings.Builder
	cmdStr.WriteString(fmt.Sprintf("echo 'select from %s/%s key %s csv hex -1' | %s", t.KbName, t.TableName, keyHex, t.ldbBinaryPath))

	cmd := exec.Command("bash", "-c", cmdStr.String())
	//log.Printf("Executing in directory %s: %s", cmd.Dir, cmdStr.String())

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return count, fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return count, fmt.Errorf("failed to start command: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		result, err := DecodeString(line, t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error decoding %v", err)
		}
		dataChan <- result
		count++
	}

	if err := scanner.Err(); err != nil {
		if closeChan {
			close(dataChan)
		}
		return count, fmt.Errorf("error reading output: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		if closeChan {
			close(dataChan)
		}
		return count, fmt.Errorf("command failed: %v", err)
	}
	if closeChan {
		close(dataChan)
	}
	return count, nil
}

func (t *TableDefinition) DumpNative(startingSector, endingSector, limit int, dataChan chan []string) (int, error) {
	if startingSector < 0 || endingSector < 0 {
		return -1, fmt.Errorf("sectors cannot be negative: starting=%d, ending=%d", startingSector, endingSector)
	}
	if startingSector > 255 || endingSector > 255 {
		return -1, fmt.Errorf("sectors must be between 0 and 255: starting=%d, ending=%d", startingSector, endingSector)
	}
	if startingSector > endingSector {
		return -1, fmt.Errorf("starting sector must be less than or equal to ending sector: starting=%d, ending=%d",
			startingSector, endingSector)
	}

	count := 0

	for sector := startingSector; sector <= endingSector; sector++ {
		if t.cached {
			r, err := t.GetDataFromCache(sector, "", dataChan)
			if r > 0 && err == nil {
				count += r
			}
		}

		var cmdStr strings.Builder
		cmdStr.WriteString(fmt.Sprintf("echo 'dump %s/%s hex -1 sector %x' | %s", t.KbName, t.TableName, sector, t.ldbBinaryPath))

		cmd := exec.Command("bash", "-c", cmdStr.String())
		//log.Printf("Executing in directory %s: %s", cmd.Dir, cmdStr.String())

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return count, fmt.Errorf("failed to create stdout pipe: %v", err)
		}

		if err := cmd.Start(); err != nil {
			return count, fmt.Errorf("failed to start command: %v", err)
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			parts := strings.Split(line, ",")
			dataChan <- parts
			t.addData2Cache(parts)
			count++
			if limit > 0 && count >= limit {
				close(dataChan)
				return count, nil
			}
		}

		if err := scanner.Err(); err != nil {
			return count, fmt.Errorf("error reading output: %v", err)
		}

		if err := cmd.Wait(); err != nil {
			return count, fmt.Errorf("command failed: %v", err)
		}
	}
	close(dataChan)
	return count, nil
}

func (t *TableDefinition) DumpNativeParallel(startingSector, endingSector, limit int, dataChan chan []string) (int, error) {
	if startingSector < 0 || endingSector < 0 {
		return -1, fmt.Errorf("sectors cannot be negative: starting=%d, ending=%d", startingSector, endingSector)
	}
	if startingSector > 255 || endingSector > 255 {
		return -1, fmt.Errorf("sectors must be between 0 and 255: starting=%d, ending=%d", startingSector, endingSector)
	}
	if startingSector > endingSector {
		return -1, fmt.Errorf("starting sector must be less than or equal to ending sector: starting=%d, ending=%d",
			startingSector, endingSector)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstError error
	count := 0

	for sector := startingSector; sector <= endingSector; sector++ {
		wg.Add(1)
		go func(sector int) {
			defer wg.Done()

			if t.cached {
				r, err := t.GetDataFromCache(sector, "", dataChan)
				if r > 0 && err == nil {
					mu.Lock()
					count += r
					mu.Unlock()
					return
				}
			}

			var cmdStr strings.Builder
			cmdStr.WriteString(fmt.Sprintf("echo 'dump %s/%s hex -1 sector %x' | %s", t.KbName, t.TableName, sector, t.ldbBinaryPath))
			cmd := exec.Command("bash", "-c", cmdStr.String())

			//log.Printf("Executing in directory %s: %s", cmd.Dir, cmdStr.String())

			stdout, err := cmd.StdoutPipe()
			if err != nil {
				mu.Lock()
				if firstError == nil {
					firstError = fmt.Errorf("failed to create stdout pipe: %v", err)
				}
				mu.Unlock()
				return
			}

			if err := cmd.Start(); err != nil {
				mu.Lock()
				if firstError == nil {
					firstError = fmt.Errorf("failed to start command: %v", err)
				}
				mu.Unlock()
				return
			}

			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}

				parts := strings.Split(line, ",")
				dataChan <- parts
				mu.Lock()
				t.addData2Cache(parts)
				count++
				if limit > 0 && count >= limit {
					close(dataChan)
					mu.Unlock()
					return
				}
				mu.Unlock()

			}

			if err := scanner.Err(); err != nil {
				mu.Lock()
				if firstError == nil {
					firstError = fmt.Errorf("error reading output: %v", err)
				}
				mu.Unlock()
				return
			}

			if err := cmd.Wait(); err != nil {
				mu.Lock()
				if firstError == nil {
					firstError = fmt.Errorf("command failed: %v", err)
				}
				mu.Unlock()
				return
			}
		}(sector)
	}

	wg.Wait()
	close(dataChan)

	if firstError != nil {
		return count, firstError
	}
	return count, nil
}
