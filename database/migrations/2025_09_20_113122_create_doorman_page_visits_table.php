<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up()
    {
        Schema::create('doorman_page_visits', function (Blueprint $table) {
            $table->id();
            $table->string('page');
            $table->string('identifier');
            $table->string('identifier_type');
            $table->date('visited_at');
            $table->timestamps();

            $table->unique(['page', 'identifier', 'identifier_type', 'visited_at'], 'unique_page_identifier_visitedat');
        });
    }

    public function down()
    {
        Schema::dropIfExists('doorman_page_visits');
    }
};
