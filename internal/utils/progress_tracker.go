// SPDX-License-Identifier: GPL-2.0-or-later
/*
 * Copyright (C) 2024 SCANOSS.COM
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 2 of the License, or
 * (at your option) any later version.
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

// Package progresstracker provides utilities for progress
package progresstracker

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// ProgressTracker tracks import progress across multiple workers.
type ProgressTracker struct {
	mu               sync.Mutex
	startTime        time.Time
	totalFiles       int
	processedFiles   int
	totalRecords     int64
	processedRecords int64
	failedFiles      int
	collectionStats  map[string]int64 // records per collection
	estimatedTotal   int64            // estimated total records
	hasEstimate      bool             // whether we have enough data for estimation

	// mpb progress bars
	progress       *mpb.Progress
	fileBar        *mpb.Bar
	recordBar      *mpb.Bar
	collectionBars map[string]*mpb.Bar
}

// NewProgressTracker creates a new progress tracker with mpb progress bars.
func NewProgressTracker(totalFiles int) *ProgressTracker {
	p := mpb.New(
		mpb.WithOutput(color.Output),
		mpb.WithAutoRefresh(),
	)

	// Create file progress bar (known total - shows visual bar with default style)
	fileBar := p.AddBar(
		int64(totalFiles),
		mpb.PrependDecorators(
			decor.Name("Files: ", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
			decor.CountersNoUnit("%d / %d", decor.WCSyncWidth),
		),
		mpb.AppendDecorators(
			decor.OnComplete(decor.Percentage(decor.WC{W: 5}), "done"),
			decor.OnComplete(
				// ETA decorator with ewma age of 30
				decor.EwmaETA(decor.ET_STYLE_GO, 30, decor.WCSyncWidth), "done",
			),
		),
	)

	// Create records progress bar (starts with unknown total, will show bar once estimated)
	recordBar := p.AddBar(0,
		mpb.PrependDecorators(
			decor.Name("Records: ", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
			decor.CurrentNoUnit("%d", decor.WCSyncWidth),
			decor.OnComplete(
				decor.Spinner(nil, decor.WCSyncSpace), "done",
			),
		),
		mpb.AppendDecorators(
			decor.AverageSpeed(0, "%.0f/s", decor.WCSyncWidth),
		),
	)

	return &ProgressTracker{
		startTime:       time.Now(),
		totalFiles:      totalFiles,
		collectionStats: make(map[string]int64),
		progress:        p,
		fileBar:         fileBar,
		recordBar:       recordBar,
		collectionBars:  make(map[string]*mpb.Bar),
	}
}

// AddRecords increments the record count and updates progress bars.
func (pt *ProgressTracker) AddRecords(count int, collectionName string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.processedRecords += int64(count)
	pt.collectionStats[collectionName] += int64(count)

	// Update record bar
	pt.recordBar.IncrBy(count)

	// Create or update collection-specific bar
	if _, exists := pt.collectionBars[collectionName]; !exists {
		// Create a new bar for this collection (unknown total - just shows count)
		collectionBar := pt.progress.AddBar(0,
			mpb.BarFillerClearOnComplete(),
			mpb.PrependDecorators(
				decor.Name("  "+collectionName+": ", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
				decor.CurrentNoUnit("%d", decor.WCSyncWidth),
				decor.OnComplete(
					decor.Spinner(nil, decor.WCSyncSpace), "done",
				),
			),
		)
		pt.collectionBars[collectionName] = collectionBar
	}

	// Update the collection bar
	bar := pt.collectionBars[collectionName]
	bar.IncrBy(count)
}

// FileCompleted marks a file as completed and updates the file progress bar.
func (pt *ProgressTracker) FileCompleted(recordCount int, success bool) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.processedFiles++
	pt.totalRecords += int64(recordCount)
	if !success {
		pt.failedFiles++
	}

	// Calculate estimated total after processing at least 10 files (or 5% of total)
	minFilesForEstimate := 10
	if pt.totalFiles < 200 {
		minFilesForEstimate = max(5, pt.totalFiles/20) // At least 5% of files
	}

	if !pt.hasEstimate && pt.processedFiles >= minFilesForEstimate {
		// Calculate average records per file
		avgRecordsPerFile := float64(pt.processedRecords) / float64(pt.processedFiles)
		pt.estimatedTotal = int64(avgRecordsPerFile * float64(pt.totalFiles))
		pt.hasEstimate = true

		// Update record bar with estimated total
		pt.recordBar.SetTotal(pt.estimatedTotal, false)
	} else if pt.hasEstimate {
		// Continuously update estimate as we get more data
		avgRecordsPerFile := float64(pt.processedRecords) / float64(pt.processedFiles)
		pt.estimatedTotal = int64(avgRecordsPerFile * float64(pt.totalFiles))
		pt.recordBar.SetTotal(pt.estimatedTotal, false)
	}

	// Update file progress bar
	pt.fileBar.Increment()
}

// MarkFileFailed increments the failed file counter and can be called separately.
func (pt *ProgressTracker) MarkFileFailed() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.failedFiles++
}

// GetFailedFiles returns the current number of failed files.
func (pt *ProgressTracker) GetFailedFiles() int {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return pt.failedFiles
}

// Wait waits for all progress bars to complete.
func (pt *ProgressTracker) Wait() {
	pt.progress.Wait()
}

// PrintFinalSummary prints the final import summary after all bars are complete.
func (pt *ProgressTracker) PrintFinalSummary() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	elapsed := time.Since(pt.startTime)
	recordsPerSec := float64(pt.processedRecords) / elapsed.Seconds()

	log.Printf("\n")
	log.Printf("╔════════════════════════════════════════════════════════════╗")
	log.Printf("║                    IMPORT COMPLETE                         ║")
	log.Printf("╚════════════════════════════════════════════════════════════╝")
	log.Printf("")

	// Files section
	if pt.failedFiles > 0 {
		log.Printf("📁 Files Processed: %d/%d (❌ %d failed)", pt.processedFiles, pt.totalFiles, pt.failedFiles)
	} else {
		log.Printf("📁 Files Processed: %d/%d (✓ all successful)", pt.processedFiles, pt.totalFiles)
	}

	// Records section
	log.Printf("📊 Total Records: %s", formatNumber(pt.totalRecords))
	log.Printf("✅ Records Inserted: %s", formatNumber(pt.processedRecords))

	// Performance section
	log.Printf("⚡ Average Speed: %.0f records/sec", recordsPerSec)
	log.Printf("⏱️  Total Time: %s", formatDuration(elapsed))

	// Collections section
	log.Printf("")
	log.Printf("📚 Records by Collection:")

	// Sort collections by count (descending)
	type collectionStat struct {
		name  string
		count int64
	}
	stats := make([]collectionStat, 0, len(pt.collectionStats))
	for name, count := range pt.collectionStats {
		stats = append(stats, collectionStat{name, count})
	}
	// Simple bubble sort
	for i := 0; i < len(stats); i++ {
		for j := i + 1; j < len(stats); j++ {
			if stats[j].count > stats[i].count {
				stats[i], stats[j] = stats[j], stats[i]
			}
		}
	}

	// Print all collections sorted by count
	for _, stat := range stats {
		percentage := float64(stat.count) / float64(pt.processedRecords) * 100
		log.Printf("   %-25s %12s  (%.1f%%)",
			stat.name+":", formatNumber(stat.count), percentage)
	}

	log.Printf("")
}

func formatNumber(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1000000:
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	case n < 1000000000:
		return fmt.Sprintf("%.2fM", float64(n)/1000000)
	}
	return fmt.Sprintf("%.2fB", float64(n)/1000000000)
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "calculating..."
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
