# frozen_string_literal: true

require 'json'
require 'open3'

# TUIFixture writes a synthetic recording for tools/live_tui_test.rb, so the live
# TUI test needs no real recording: a 540-cue WebVTT transcript, a talk_meta.json
# with chapters, and a tiny generated video whose duration covers the transcript.
#
# The values the test asserts on are fixed here: cue 1 (Boris Aronov, "Go ahead."),
# cue 2 (Adam Sheffer, "Great!"), cue 9 at 00:39 (Karim Abu Affash) opening the
# "Introduction" chapter, and cue 12 at 01:02 opening "Definitions and Related Work",
# with cue 13 at 01:09.
module TUIFixture
  CUE_COUNT = 540
  STEP = 5.0 # seconds between cues after the opening ones
  SPEAKER = 'Karim Abu Affash'
  OPENING = [
    [0.0, 'Boris Aronov', 'Go ahead.'],
    [2.0, 'Adam Sheffer', 'Great!'],
    [6.0, 'Adam Sheffer', 'Welcome everyone to the seminar.'],
    [11.0, 'Adam Sheffer', 'Today we are happy to host our speaker.'],
    [16.0, 'Adam Sheffer', 'He will talk about oriented spanners.'],
    [21.0, 'Adam Sheffer', 'Please keep your microphones muted.'],
    [26.0, 'Adam Sheffer', 'Questions are welcome at the end.'],
    [31.0, 'Adam Sheffer', 'Karim, the floor is yours.'],
    [39.0, SPEAKER, 'Thank you for the kind introduction.'],
    [46.0, SPEAKER, 'This is joint work with several colleagues.'],
    [54.0, SPEAKER, 'Let me start with the basic setting.'],
    [62.0, SPEAKER, 'First we recall some definitions.'],
    [69.0, SPEAKER, 'A spanner approximates all pairwise distances.']
  ].freeze
  CHAPTERS = [
    [39.0, 'Introduction'],
    [62.0, 'Definitions and Related Work'],
    [900.0, 'Main Results'],
    [2400.0, 'Open Problems']
  ].freeze

  module_function

  # Writes the fixture into dir and returns dir.
  def write(dir)
    File.write(File.join(dir, 'recording.transcript.vtt'), vtt)
    File.write(File.join(dir, 'talk_meta.json'), JSON.pretty_generate(meta))
    write_video(File.join(dir, 'recording.mp4'), cues.last[0] + STEP)
    dir
  end

  def cues
    @cues ||= begin
      list = OPENING.dup
      last = OPENING.last[0]
      (OPENING.size + 1..CUE_COUNT).each do |n|
        list << [last + ((n - OPENING.size) * STEP), SPEAKER, "Point #{n} of the argument about spanners."]
      end
      list
    end
  end

  def vtt
    body = cues.each_with_index.map do |(start, speaker, text), i|
      finish = i + 1 < cues.size ? cues[i + 1][0] - 0.5 : start + STEP - 0.5
      "#{i + 1}\n#{stamp(start)} --> #{stamp(finish)}\n#{speaker}: #{text}\n"
    end
    "WEBVTT\n\n#{body.join("\n")}"
  end

  def meta
    {
      'title' => 'Oriented Spanners in Metric Spaces',
      'speaker' => SPEAKER,
      'abstract' => 'A synthetic talk used by the live TUI test.',
      'privacy' => 'unlisted',
      'chapters' => CHAPTERS.map do |sec, title|
        ns = (sec * 1_000_000_000).to_i
        { 'original_time' => ns, 'adjusted_time' => ns, 'title' => title }
      end
    }
  end

  def stamp(sec)
    whole = sec.floor
    millis = ((sec - whole) * 1000).round
    format('%<h>02d:%<m>02d:%<s>02d.%<ms>03d', h: whole / 3600, m: (whole / 60) % 60, s: whole % 60, ms: millis)
  end

  # A one-frame-per-second 64x36 black video with silent audio: small and fast to make.
  def write_video(path, seconds)
    duration = seconds.ceil.to_s
    cmd = ['ffmpeg', '-v', 'error', '-y',
           '-f', 'lavfi', '-i', "color=c=black:s=64x36:r=1:d=#{duration}",
           '-f', 'lavfi', '-i', 'anullsrc=r=8000:cl=mono',
           '-t', duration, '-c:v', 'libx264', '-preset', 'ultrafast', '-c:a', 'aac', '-shortest', path]
    _, err, status = Open3.capture3(*cmd)
    abort "TUIFixture: ffmpeg failed to make #{path}: #{err}" unless status.success?
  end
end
