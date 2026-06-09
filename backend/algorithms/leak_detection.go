package algorithms

import (
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
)

type DetectorReading struct {
	DeviceID      string
	Position      float64
	Latitude      float64
	Longitude     float64
	Concentration float64
	Timestamp     time.Time
}

type Particle struct {
	Position     float64
	LeakRate     float64
	Velocity     float64
	VelocityRate float64
	BestPos      float64
	BestRate     float64
	BestFitness  float64
}

type LeakSourceResult struct {
	Position        float64
	Latitude        float64
	Longitude       float64
	LeakRate        float64
	Confidence      float64
	DiffusionRadius float64
}

type PSOConfig struct {
	NumParticles     int
	MaxIterations    int
	InertiaWeight    float64
	CognitiveWeight  float64
	SocialWeight     float64
	SearchRangeMin   float64
	SearchRangeMax   float64
	LeakRateMin      float64
	LeakRateMax      float64
}

type GaussianPlumeModel struct {
	WindSpeed            float64
	WindDir              float64
	Temperature          float64
	AtmosphericStability float64
}

type DataQualityReport struct {
	ValidReadings      int
	TotalReadings      int
	WindSpeedValid     bool
	WindDirValid       bool
	ConcentrationRange float64
	QualityScore       float64
	DegradedMode       bool
	DegradationReason  string
}

type AlgorithmType string

const (
	AlgorithmPSO   AlgorithmType = "pso"
	AlgorithmBayes AlgorithmType = "bayes"
)

func (m *GaussianPlumeModel) CalculateConcentration(sourcePos, detectorPos, leakRate float64) float64 {
	distance := math.Abs(sourcePos - detectorPos)

	if distance < 1.0 {
		return leakRate * 0.1
	}

	sigmaY := 0.22 * distance / math.Pow(1+0.0001*distance, 0.5)
	sigmaZ := 0.2 * distance

	windFactor := 1.0
	if m.WindSpeed > 0.1 {
		windFactor = 1.0 / (m.WindSpeed * math.Sqrt(2*math.Pi))
	}

	concentration := leakRate * windFactor *
		math.Exp(-0.5*math.Pow(distance/sigmaY, 2)) *
		math.Exp(-0.5*math.Pow(1.5/sigmaZ, 2)) / (sigmaY * sigmaZ * math.Sqrt(2*math.Pi))

	return concentration * 1000
}

func DefaultPSOConfig() PSOConfig {
	return PSOConfig{
		NumParticles:    50,
		MaxIterations:   100,
		InertiaWeight:   0.7,
		CognitiveWeight: 1.5,
		SocialWeight:    1.5,
		SearchRangeMin:  0,
		SearchRangeMax:  30000,
		LeakRateMin:     0.001,
		LeakRateMax:     10.0,
	}
}

func (p *PSOConfig) LoadFromConfig(cfg *config.LeakDetectionConfig) {
	if cfg.PSOParticles > 0 {
		p.NumParticles = cfg.PSOParticles
	}
	if cfg.PSOIterations > 0 {
		p.MaxIterations = cfg.PSOIterations
	}
	if cfg.PSOInertiaWeight > 0 {
		p.InertiaWeight = cfg.PSOInertiaWeight
	}
	if cfg.PSOCognitiveWeight > 0 {
		p.CognitiveWeight = cfg.PSOCognitiveWeight
	}
	if cfg.PSOSocialWeight > 0 {
		p.SocialWeight = cfg.PSOSocialWeight
	}
}

func fitnessFunction(readings []DetectorReading, sourcePos, leakRate float64, model *GaussianPlumeModel) float64 {
	totalError := 0.0
	count := 0

	for _, reading := range readings {
		if reading.Concentration < 0.1 {
			continue
		}

		expected := model.CalculateConcentration(sourcePos, reading.Position, leakRate)
		error := math.Abs(reading.Concentration - expected)
		totalError += error * error
		count++
	}

	if count == 0 {
		return math.Inf(1)
	}

	return math.Sqrt(totalError / float64(count))
}

func LocalizeLeakSource(readings []DetectorReading, model *GaussianPlumeModel, psoCfg PSOConfig) (*LeakSourceResult, error) {
	if len(readings) < 3 {
		return nil, nil
	}

	validReadings := make([]DetectorReading, 0, len(readings))
	maxConc := 0.0
	for _, r := range readings {
		if r.Concentration > 0.0 {
			validReadings = append(validReadings, r)
			if r.Concentration > maxConc {
				maxConc = r.Concentration
			}
		}
	}

	if len(validReadings) < 3 || maxConc < 1.0 {
		return nil, nil
	}

	rand.Seed(time.Now().UnixNano())

	particles := make([]Particle, psoCfg.NumParticles)
	globalBest := Particle{BestFitness: math.Inf(1)}

	searchCenter := estimateLeakPosition(validReadings)
	searchRadius := 1000.0

	for i := range particles {
		pos := searchCenter + (rand.Float64()-0.5)*2*searchRadius
		pos = math.Max(psoCfg.SearchRangeMin, math.Min(psoCfg.SearchRangeMax, pos))

		rate := psoCfg.LeakRateMin + rand.Float64()*(psoCfg.LeakRateMax-psoCfg.LeakRateMin)

		fitness := fitnessFunction(validReadings, pos, rate, model)

		particles[i] = Particle{
			Position:     pos,
			LeakRate:     rate,
			Velocity:     (rand.Float64() - 0.5) * 20,
			VelocityRate: (rand.Float64() - 0.5) * 0.1,
			BestPos:      pos,
			BestRate:     rate,
			BestFitness:  fitness,
		}

		if fitness < globalBest.BestFitness {
			globalBest = particles[i]
		}
	}

	var wg sync.WaitGroup
	for iter := 0; iter < psoCfg.MaxIterations; iter++ {
		wg.Add(psoCfg.NumParticles)
		for i := range particles {
			go func(idx int) {
				defer wg.Done()

				r1, r2 := rand.Float64(), rand.Float64()

				particles[idx].Velocity = psoCfg.InertiaWeight*particles[idx].Velocity +
					psoCfg.CognitiveWeight*r1*(particles[idx].BestPos-particles[idx].Position) +
					psoCfg.SocialWeight*r2*(globalBest.Position-particles[idx].Position)

				particles[idx].VelocityRate = psoCfg.InertiaWeight*particles[idx].VelocityRate +
					psoCfg.CognitiveWeight*r1*(particles[idx].BestRate-particles[idx].LeakRate) +
					psoCfg.SocialWeight*r2*(globalBest.LeakRate-particles[idx].LeakRate)

				particles[idx].Position += particles[idx].Velocity
				particles[idx].LeakRate += particles[idx].VelocityRate

				particles[idx].Position = math.Max(psoCfg.SearchRangeMin, math.Min(psoCfg.SearchRangeMax, particles[idx].Position))
				particles[idx].LeakRate = math.Max(psoCfg.LeakRateMin, math.Min(psoCfg.LeakRateMax, particles[idx].LeakRate))

				fitness := fitnessFunction(validReadings, particles[idx].Position, particles[idx].LeakRate, model)

				if fitness < particles[idx].BestFitness {
					particles[idx].BestFitness = fitness
					particles[idx].BestPos = particles[idx].Position
					particles[idx].BestRate = particles[idx].LeakRate
				}
			}(i)
		}
		wg.Wait()

		for i := range particles {
			if particles[i].BestFitness < globalBest.BestFitness {
				globalBest = particles[i]
			}
		}
	}

	lat, lon := positionToLatLon(globalBest.BestPos, validReadings)

	confidence := calculateConfidence(globalBest.BestFitness, maxConc)
	diffusionRadius := calculateDiffusionRadius(globalBest.LeakRate, model.WindSpeed, config.AppConfig.LeakDetection.DiffusionRadiusBase)

	return &LeakSourceResult{
		Position:        globalBest.BestPos,
		Latitude:        lat,
		Longitude:       lon,
		LeakRate:        globalBest.LeakRate,
		Confidence:      confidence,
		DiffusionRadius: diffusionRadius,
	}, nil
}

func estimateLeakPosition(readings []DetectorReading) float64 {
	weightedSum := 0.0
	weightTotal := 0.0

	for _, r := range readings {
		weight := r.Concentration * r.Concentration
		weightedSum += r.Position * weight
		weightTotal += weight
	}

	if weightTotal > 0 {
		return weightedSum / weightTotal
	}

	return 15000
}

func positionToLatLon(position float64, readings []DetectorReading) (float64, float64) {
	if len(readings) == 0 {
		return 39.9042, 116.4074
	}

	var closest DetectorReading
	minDist := math.Inf(1)
	var prev, next DetectorReading

	for _, r := range readings {
		dist := math.Abs(r.Position - position)
		if dist < minDist {
			minDist = dist
			closest = r
		}
	}

	foundPrev, foundNext := false, false
	for _, r := range readings {
		if r.Position <= position && (!foundPrev || r.Position > prev.Position) {
			prev = r
			foundPrev = true
		}
		if r.Position >= position && (!foundNext || r.Position < next.Position) {
			next = r
			foundNext = true
		}
	}

	if !foundPrev {
		return next.Latitude, next.Longitude
	}
	if !foundNext {
		return prev.Latitude, prev.Longitude
	}

	if next.Position == prev.Position {
		return prev.Latitude, prev.Longitude
	}

	ratio := (position - prev.Position) / (next.Position - prev.Position)
	lat := prev.Latitude + ratio*(next.Latitude-prev.Latitude)
	lon := prev.Longitude + ratio*(next.Longitude-prev.Longitude)

	return lat, lon
}

func calculateConfidence(fitness, maxConc float64) float64 {
	normalizedFitness := fitness / (maxConc + 1e-6)
	confidence := math.Exp(-normalizedFitness * 2)
	confidence = math.Max(0, math.Min(1, confidence))
	return confidence * 100
}

func calculateDiffusionRadius(leakRate, windSpeed, baseRadius float64) float64 {
	radius := baseRadius * math.Sqrt(leakRate)

	windFactor := 1.0
	if windSpeed > 0.5 {
		windFactor = 1.0 + windSpeed*0.3
	}

	return radius * windFactor
}

func AssessDataQuality(readings []DetectorReading, model *GaussianPlumeModel) DataQualityReport {
	report := DataQualityReport{
		TotalReadings:  len(readings),
		WindSpeedValid: model.WindSpeed >= 0 && model.WindSpeed < 50,
		WindDirValid:   model.WindDir >= 0 && model.WindDir <= 360,
	}

	validCount := 0
	maxConc := 0.0
	minConc := math.MaxFloat64

	for _, r := range readings {
		if r.Concentration >= 0 && r.Concentration < 1000 {
			validCount++
			if r.Concentration > maxConc {
				maxConc = r.Concentration
			}
			if r.Concentration < minConc {
				minConc = r.Concentration
			}
		}
	}

	report.ValidReadings = validCount
	report.ConcentrationRange = maxConc - minConc

	validRatio := float64(validCount) / float64(len(readings))
	report.QualityScore = validRatio * 100

	if !report.WindSpeedValid || !report.WindDirValid {
		report.QualityScore *= 0.5
		report.DegradedMode = true
		if !report.WindSpeedValid && !report.WindDirValid {
			report.DegradationReason = "风速风向传感器全部故障"
		} else if !report.WindSpeedValid {
			report.DegradationReason = "风速传感器故障"
		} else {
			report.DegradationReason = "风向传感器故障"
		}
	}

	if validCount < 5 {
		report.DegradedMode = true
		report.DegradationReason = "有效读数不足"
		report.QualityScore *= 0.5
	}

	if report.ConcentrationRange < 5.0 {
		report.DegradedMode = true
		report.DegradationReason = "浓度梯度不足"
		report.QualityScore *= 0.7
	}

	return report
}

func SelectAlgorithm(quality DataQualityReport) AlgorithmType {
	if quality.DegradedMode {
		log.Printf("数据质量降级: %s, 质量分数: %.1f%%, 自动切换到PSO算法", 
			quality.DegradationReason, quality.QualityScore)
		return AlgorithmPSO
	}

	if quality.QualityScore >= 70 {
		return AlgorithmBayes
	}

	return AlgorithmPSO
}

func LocalizeLeakSourceWithQualityCheck(readings []DetectorReading, model *GaussianPlumeModel, psoCfg PSOConfig) (*LeakSourceResult, DataQualityReport, error) {
	quality := AssessDataQuality(readings, model)

	if quality.ValidReadings < 3 {
		return nil, quality, nil
	}

	algorithm := SelectAlgorithm(quality)

	var result *LeakSourceResult
	var err error

	if algorithm == AlgorithmBayes {
		log.Printf("使用贝叶斯推断算法, 数据质量: %.1f%%", quality.QualityScore)
		result, err = BayesInference(readings, model)
		
		if err != nil || result == nil {
			log.Printf("贝叶斯推断失败或无结果, 降级使用PSO算法")
			result, err = LocalizeLeakSource(readings, model, psoCfg)
		} else if result.Confidence < 30 {
			log.Printf("贝叶斯推断置信度过低 (%.1f%%), 降级使用PSO算法验证", result.Confidence)
			psoResult, psoErr := LocalizeLeakSource(readings, model, psoCfg)
			if psoErr == nil && psoResult != nil && psoResult.Confidence > result.Confidence {
				result = psoResult
				log.Printf("PSO算法结果更优, 置信度: %.1f%%", result.Confidence)
			}
		}
	} else {
		log.Printf("使用PSO算法, 数据质量: %.1f%%", quality.QualityScore)
		result, err = LocalizeLeakSource(readings, model, psoCfg)
	}

	return result, quality, err
}

func BayesInference(readings []DetectorReading, model *GaussianPlumeModel) (*LeakSourceResult, error) {
	if len(readings) < 3 {
		return nil, nil
	}

	validReadings := make([]DetectorReading, 0, len(readings))
	maxConc := 0.0
	for _, r := range readings {
		if r.Concentration >= 0.0 && r.Concentration < 1000 {
			validReadings = append(validReadings, r)
			if r.Concentration > maxConc {
				maxConc = r.Concentration
			}
		}
	}

	if len(validReadings) < 3 || maxConc < 1.0 {
		return nil, nil
	}

	estimatedCenter := estimateLeakPosition(validReadings)
	searchRadius := math.Max(500.0, 3000.0*(maxConc/100.0))
	searchMin := math.Max(0, estimatedCenter-searchRadius)
	searchMax := math.Min(30000, estimatedCenter+searchRadius)

	gridResolution := 50.0
	leakRateResolution := 0.05

	maxProb := 0.0
	bestPos := estimatedCenter
	bestRate := 1.0

	totalPositions := int((searchMax - searchMin) / gridResolution)
	totalRates := int(10.0 / leakRateResolution)

	if totalPositions < 5 {
		totalPositions = 5
	}

	logProb := 0.0
	maxLogProb := math.Inf(-1)

	for i := 0; i < totalPositions; i++ {
		pos := searchMin + float64(i)*gridResolution
		for j := 0; j < totalRates; j++ {
			rate := float64(j+1) * leakRateResolution

			logLikelihood := 0.0
			validCount := 0
			for _, r := range validReadings {
				if r.Concentration <= 0.0 {
					continue
				}

				expected := model.CalculateConcentration(pos, r.Position, rate)
				if expected < 0 {
					expected = 0
				}
				error := r.Concentration - expected
				stdDev := math.Max(0.5, r.Concentration*0.1)
				logLikelihood += -0.5*error*error/(stdDev*stdDev) - math.Log(stdDev*math.Sqrt(2*math.Pi))
				validCount++
			}

			if validCount < 3 {
				continue
			}

			logProb = logLikelihood

			if logProb > maxLogProb {
				maxLogProb = logProb
				bestPos = pos
				bestRate = rate
				maxProb = math.Exp(logProb)
			}
		}
	}

	if maxLogProb == math.Inf(-1) || maxProb < 1e-20 {
		return nil, nil
	}

	var posSum, rateSum, probSum float64
	for i := 0; i < totalPositions; i++ {
		pos := searchMin + float64(i)*gridResolution
		for j := 0; j < totalRates; j++ {
			rate := float64(j+1) * leakRateResolution

			logLikelihood := 0.0
			validCount := 0
			for _, r := range validReadings {
				if r.Concentration <= 0.0 {
					continue
				}

				expected := model.CalculateConcentration(pos, r.Position, rate)
				if expected < 0 {
					expected = 0
				}
				error := r.Concentration - expected
				stdDev := math.Max(0.5, r.Concentration*0.1)
				logLikelihood += -0.5*error*error/(stdDev*stdDev) - math.Log(stdDev*math.Sqrt(2*math.Pi))
				validCount++
			}

			if validCount < 3 {
				continue
			}

			prob := math.Exp(logLikelihood - maxLogProb)
			posSum += pos * prob
			rateSum += rate * prob
			probSum += prob
		}
	}

	if probSum > 1e-10 {
		bestPos = posSum / probSum
		bestRate = rateSum / probSum
	}

	bestPos = math.Max(0, math.Min(30000, bestPos))
	bestRate = math.Max(0.001, math.Min(10.0, bestRate))

	if math.Abs(bestPos-estimatedCenter) > searchRadius*2 {
		log.Printf("贝叶斯推断结果偏离估计中心过远 (%.0fm vs %.0fm), 使用估计中心", bestPos, estimatedCenter)
		bestPos = estimatedCenter
	}

	lat, lon := positionToLatLon(bestPos, validReadings)
	confidence := math.Min(100, -maxLogProb/100.0)
	if confidence < 0 {
		confidence = 50.0
	}
	diffusionRadius := calculateDiffusionRadius(bestRate, model.WindSpeed, config.AppConfig.LeakDetection.DiffusionRadiusBase)

	return &LeakSourceResult{
		Position:        bestPos,
		Latitude:        lat,
		Longitude:       lon,
		LeakRate:        bestRate,
		Confidence:      confidence,
		DiffusionRadius: diffusionRadius,
	}, nil
}
