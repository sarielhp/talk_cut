#!/usr/bin/env ruby
# frozen_string_literal: true

require 'open3'
require 'set'

ENV['PATH'] = "#{File.expand_path('~/.go/bin')}:#{File.expand_path('~/go/bin')}:#{File.expand_path('~/prog/standards/go/bin')}:#{File.expand_path('~/bin')}:#{ENV['PATH']}"
root_dir = File.expand_path('..', __dir__)
Dir.chdir(root_dir)

proj_name = 'talk_cut'

def run_cmd(name, cmd)
  stdout, stderr, status = Open3.capture3(cmd)
  unless status.success?
    puts "\e[31m✗ #{name} failed!\e[0m"
    puts "\e[1mStdout:\e[0m" unless stdout.empty?
    puts stdout unless stdout.empty?
    puts "\e[1mStderr:\e[0m" unless stderr.empty?
    puts stderr unless stderr.empty?
    exit 1
  end
  [stdout, stderr]
end

puts "\e[1m=== Quality Gate (#{proj_name}) ===\e[0m"

# 1. Format
print '1. Checking format (gofmt -s)... '
unformatted_out, _ = Open3.capture3('gofmt -s -l .')
unformatted = unformatted_out.lines.map(&:strip).reject(&:empty?)
if unformatted.any?
  _, err, status = Open3.capture3('gofmt -s -w .')
  if !status.success?
    puts "\n\e[31m✗ Formatting failed:\e[0m\n#{err}"
    exit 1
  else
    puts "\e[32m✔\e[0m (formatted #{unformatted.size} file(s))"
  end
else
  puts "\e[32m✔\e[0m"
end

# 2. Mod tidy
if File.exist?('go.mod')
  print '2. Checking go.mod cleanliness... '
  run_cmd('Go mod tidy', 'go mod tidy')
  puts "\e[32m✔\e[0m"
end

# 3. Vet
print '3. Running go vet... '
run_cmd('Go vet', 'go vet ./...')
puts "\e[32m✔\e[0m"

# 4. Staticcheck (if installed)
staticcheck_bin = `which staticcheck 2>/dev/null`.strip
if !staticcheck_bin.empty?
  print '4. Running staticcheck... '
  out, _, _ = Open3.capture3('staticcheck ./...')
  baseline_file = File.join(root_dir, 'tools', 'staticcheck-baseline.txt')
  if File.exist?(baseline_file)
    baseline_lines = File.read(baseline_file).lines.map(&:strip)
    normalize_issue = ->(l) { l.sub(/:\d+:\d+:/, ':') }
    baseline_set = baseline_lines.map(&normalize_issue).to_set
    current_lines = out.lines.map(&:strip).reject(&:empty?)
    new_issues = current_lines.reject { |l| baseline_set.include?(normalize_issue.call(l)) }
    if new_issues.any?
      puts "\n\e[31m✗ Staticcheck found #{new_issues.size} new issue(s):\e[0m"
      puts new_issues.join("\n")
      exit 1
    end
  elsif !out.strip.empty?
    puts "\n\e[31m✗ Staticcheck found issue(s):\e[0m\n#{out}"
    exit 1
  end
  puts "\e[32m✔\e[0m"
end

# 5. Cognitive complexity and line audit
print '5. Auditing cognitive complexity (go-audit)... '
audit_bin = `which go-audit 2>/dev/null`.strip
audit_cmd = if !audit_bin.empty?
              'go-audit --strict --quiet'
            elsif File.exist?(File.expand_path('~/prog/standards/go/bin/go-audit'))
              "#{File.expand_path('~/prog/standards/go/bin/go-audit')} --strict --quiet"
            end

if audit_cmd
  stdout, _ = run_cmd('Line & complexity audit', audit_cmd)
  puts "\e[32m✔\e[0m"
  puts stdout.strip unless stdout.strip.empty?
else
  puts "\e[33m⚠ (go-audit not found in PATH)\e[0m"
end

# 6. Go test
print '6. Running unit tests (go test)... '
run_cmd('Go test', 'go test ./...')
puts "\e[32m✔\e[0m"

# 7. Build
print '7. Building binary check... '
out, err, s = Open3.capture3('go build -o /dev/null .')
unless s.success?
  puts "\n\e[31m✗ Build failed!\e[0m\n#{err}"
  exit 1
end
puts "\e[32m✔\e[0m"

# 8. Semantic version validation
version_file = File.join(root_dir, 'VERSION')
if File.exist?(version_file)
  print '8. Validating semantic version... '
  ver = File.read(version_file).strip
  unless ver =~ /\A\d+\.\d+\.\d+\z/
    puts "\n\e[31m✗ Invalid semantic version in VERSION: #{ver}\e[0m"
    exit 1
  end
  puts "\e[32m✔\e[0m (#{ver})"
end

puts "\n\e[32m✔ All quality gates passed successfully!\e[0m"
exit 0
